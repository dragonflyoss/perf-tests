/*
 *     Copyright 2025 The Dragonfly Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	name    = "honk"
	version = "1.2.0"
)

var rootCmd = &cobra.Command{
	Use:     name,
	Version: version,
	Short:   "Show stock real-time data tools",
	Long: `A command line tool to display real-time stock 
information and analysis results.
Complete documentation is available at https://github.com/gaius-qi/honk`,
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, cancel := context.WithCancel(context.Background())
		defer cancel()

		return nil
	},
}

// Execute is the entry point of the command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logrus.Debugf("Execute error: %#v", err)
	}
}
