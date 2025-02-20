/*
Copyright 2023 Tim Ebert.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pager

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultPageSize       = 500
	defaultPageBufferSize = 10
)

// lister is the subset of client.Reader that ListPager uses.
type lister interface {
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

// New creates a new pager from the provided reader using the default options.
func New(reader lister) *ListPager {
	return &ListPager{
		Reader:         reader,
		PageSize:       defaultPageSize,
		PageBufferSize: defaultPageBufferSize,
	}
}

// ListPager assists client code in breaking large list queries into multiple smaller chunks of PageSize or smaller.
// The pager does not alter any fields on the initial options list except Continue.
// It is implemented like client-go's pager but uses a controller-runtime client,
// see https://github.com/kubernetes/client-go/blob/release-1.28/tools/pager/pager.go.
// Exception: this ListPager also fixes the `specifying resource version is not allowed when using continue` error
// in EachListItem and EachListItemWithAlloc.
type ListPager struct {
	Reader lister

	// PageSize is the maximum number of objects to retrieve in individual list calls.
	// If a client.Limit option is passed, the pager uses the option's value instead.
	PageSize int64
	// Number of pages to buffer in EachListItem and EachListItemWithAlloc.
	PageBufferSize int32
}

// EachListItem fetches runtime.Object items using this ListPager and invokes fn on each item. If
// fn returns an error, processing stops and that error is returned. If fn does not return an error,
// any error encountered while retrieving the list from the server is returned. If the context
// cancels or times out, the context error is returned. Since the list is retrieved in paginated
// chunks, an "Expired" error (metav1.StatusReasonExpired) may be returned if the pagination list
// requests exceed the expiration limit of the apiserver being called.
//
// Items are retrieved in chunks from the server to reduce the impact on the server with up to
// ListPager.PageBufferSize chunks buffered concurrently in the background.
//
// If items passed to fn are retained for different durations, and you want to avoid
// retaining the whole slice returned by p.Reader.List as long as any item is referenced,
// use EachListItemWithAlloc instead.
func (p *ListPager) EachListItem(ctx context.Context, list client.ObjectList, fn func(obj client.Object) error, opts ...client.ListOption) error {
	return p.eachListChunkBuffered(ctx, list, func(list client.ObjectList) error {
		return meta.EachListItem(list, func(obj runtime.Object) error {
			return fn(obj.(client.Object))
		})
	}, opts...)
}

// EachListItemWithAlloc works like EachListItem, but avoids retaining references to the items slice returned by p.Reader.List.
// It does this by making a shallow copy of non-pointer items in the slice returned by p.Reader.List.
//
// If the items passed to fn are not retained, or are retained for the same duration, use EachListItem instead for memory efficiency.
func (p *ListPager) EachListItemWithAlloc(ctx context.Context, list client.ObjectList, fn func(obj client.Object) error, opts ...client.ListOption) error {
	return p.eachListChunkBuffered(ctx, list, func(list client.ObjectList) error {
		return meta.EachListItemWithAlloc(list, func(obj runtime.Object) error {
			return fn(obj.(client.Object))
		})
	}, opts...)
}

// eachListChunkBuffered fetches runtimeObject list chunks using this ListPager and invokes fn on
// each list chunk.  If fn returns an error, processing stops and that error is returned. If fn does
// not return an error, any error encountered while retrieving the list from the server is
// returned. If the context cancels or times out, the context error is returned. Since the list is
// retrieved in paginated chunks, an "Expired" error (metav1.StatusReasonExpired) may be returned if
// the pagination list requests exceed the expiration limit of the apiserver being called.
//
// Up to ListPager.PageBufferSize chunks are buffered concurrently in the background.
func (p *ListPager) eachListChunkBuffered(ctx context.Context, list client.ObjectList, fn func(list client.ObjectList) error, opts ...client.ListOption) error {
	if p.PageBufferSize < 0 {
		return fmt.Errorf("ListPager.PageBufferSize must be >= 0, got %d", p.PageBufferSize)
	}

	// Ensure background goroutine is stopped if this call exits before all list items are
	// processed. Cancellation error from this deferred cancel call is never returned to caller;
	// either the list result has already been sent to bgResultC or the fn error is returned and
	// the cancellation error is discarded.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	chunkC := make(chan client.ObjectList, p.PageBufferSize)
	bgResultC := make(chan error, 1)
	go func() {
		defer utilruntime.HandleCrash()

		var err error
		defer func() {
			close(chunkC)
			bgResultC <- err
		}()
		err = p.eachListChunk(ctx, list, func(chunk client.ObjectList) error {
			select {
			case chunkC <- chunk: // buffer the chunk, this can block
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		}, opts...)
	}()

	for o := range chunkC {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		err := fn(o)
		if err != nil {
			return err // any fn error should be returned immediately
		}
	}
	// promote the results of our background goroutine to the foreground
	return <-bgResultC
}

// eachListChunk fetches runtimeObject list chunks using this ListPager and invokes fn on each list
// chunk. If fn returns an error, processing stops and that error is returned. If fn does not return
// an error, any error encountered while retrieving the list from the server is returned. If the
// context cancels or times out, the context error is returned. Since the list is retrieved in
// paginated chunks, an "Expired" error (metav1.StatusReasonExpired) may be returned if the
// pagination list requests exceed the expiration limit of the apiserver being called.
func (p *ListPager) eachListChunk(ctx context.Context, list client.ObjectList, fn func(list client.ObjectList) error, opts ...client.ListOption) error {
	options := &client.ListOptions{}
	options.ApplyOptions(opts)

	if options.Limit == 0 {
		options.Limit = p.PageSize
	}
	if options.Limit < 0 {
		return fmt.Errorf("ListPager.PageSize must be >= 0, got %d", options.Limit)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// create a new list to not share memory between paginated lists
		list = list.DeepCopyObject().(client.ObjectList)
		if err := p.Reader.List(ctx, list, options); err != nil {
			return err
		}

		if err := fn(list); err != nil {
			return err
		}

		// if we have no more items, return.
		if len(list.GetContinue()) == 0 {
			return nil
		}

		// set the next loop up
		options.Continue = list.GetContinue()
		// Clear the ResourceVersion(Match) on the subsequent List calls to avoid the
		// `specifying resource version is not allowed when using continue` error.
		// See https://github.com/kubernetes/kubernetes/issues/85221#issuecomment-553748143.
		if options.Raw == nil {
			options.Raw = &metav1.ListOptions{}
		}
		options.Raw.ResourceVersion = ""
		options.Raw.ResourceVersionMatch = ""
	}
}
