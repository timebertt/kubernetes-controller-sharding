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

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

func main() {
	var (
		maxAge time.Duration
	)

	cmd := &cobra.Command{
		Use:   "janitor path",
		Short: "janitor is a stupidly simple storage retention tool",
		Long:  `janitor deletes everything in the given directory that has a modification timestamp older than a given retention time.`,

		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if path == "" {
				return fmt.Errorf("path must not be empty")
			}
			if maxAge <= 0 {
				return fmt.Errorf("max-age must be greater than 0")
			}

			cmd.SilenceUsage = true

			return run(path, time.Now().Add(-maxAge))
		},
	}

	cmd.Flags().DurationVar(&maxAge, "max-age", 0, "Maximum age of files and directories to keep. Elements with an older modification timestamp are deleted.")

	if err := cmd.ExecuteContext(signals.SetupSignalHandler()); err != nil {
		log.Fatalln(err)
	}
}

func run(dir string, cleanBefore time.Time) error {
	performedCleanup := false

	elements, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, element := range elements {
		path := filepath.Join(dir, element.Name())

		info, err := element.Info()
		if err != nil {
			return fmt.Errorf("failed to get info for %q: %w", path, err)
		}

		modTime := info.ModTime()
		if !modTime.Before(cleanBefore) {
			continue
		}

		log.Printf("Cleaning %s (last modified: %s)", path, modTime.UTC().Format(time.RFC3339))

		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("failed to remove %q: %w", path, err)
		}
		performedCleanup = true
	}

	if !performedCleanup {
		log.Println("Nothing to clean")
	}

	return nil
}
