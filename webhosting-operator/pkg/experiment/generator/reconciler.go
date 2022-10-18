/*
Copyright 2022 Tim Ebert.

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

package generator

import (
	"context"
	"reflect"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Every runs the given Func with the specified frequency.
type Every struct {
	client.Client

	Name   string
	Do     func(ctx context.Context, c client.Client, labels map[string]string) error
	Every  time.Duration
	Stop   time.Time
	Labels map[string]string
}

func (r *Every) AddToManager(mgr manager.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}

	c, err := controller.New(r.Name, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: 10,
		RecoverPanic:            true,
	})
	if err != nil {
		return err
	}

	return StartOnce(c)
}

func (r *Every) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	if !r.Stop.IsZero() && !time.Now().Before(r.Stop) {
		// stop now
		return reconcile.Result{}, nil
	}

	return reconcile.Result{RequeueAfter: r.Every}, r.Do(ctx, r.Client, r.Labels)
}

// ForEach runs the given Func for each object of the given kind with the specified frequency.
type ForEach[T client.Object] struct {
	client.Client

	Name   string
	Do     func(ctx context.Context, c client.Client, obj T) error
	Every  time.Duration
	Stop   time.Time
	Labels map[string]string

	gvk schema.GroupVersionKind
	obj T
}

func (r *ForEach[T]) AddToManager(mgr manager.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}

	var t T
	r.obj = reflect.New(reflect.TypeOf(t).Elem()).Interface().(T)

	var err error
	r.gvk, err = apiutil.GVKForObject(r.obj, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := controller.New(r.Name, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: 10,
		RecoverPanic:            true,
	})
	if err != nil {
		return err
	}

	return c.Watch(
		&source.Kind{Type: r.obj},
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc:  func(event.CreateEvent) bool { return true },
			DeleteFunc:  func(event.DeleteEvent) bool { return false },
			UpdateFunc:  func(event.UpdateEvent) bool { return false },
			GenericFunc: func(event.GenericEvent) bool { return false },
		},
	)
}

func (r *ForEach[T]) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	if !r.Stop.IsZero() && !time.Now().Before(r.Stop) {
		// stop now
		return reconcile.Result{}, nil
	}

	obj := r.obj.DeepCopyObject().(T)
	if err := r.Get(ctx, request.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: r.Every}, r.Do(ctx, r.Client, obj)
}
