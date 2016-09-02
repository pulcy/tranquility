// Copyright (c) 2016 Pulcy.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"github.com/spf13/cobra"

	"github.com/pulcy/tranquility/service"
)

var (
	fixCmd = &cobra.Command{
		Use:   "fix",
		Short: "Fix invalid fleet units.",
		Run:   fixCmdRun,
	}
)

func fixCmdRun(cmd *cobra.Command, args []string) {
	fleet, err := newFleet()
	if err != nil {
		Exitf("Failed to create fleet accessor: %#v", err)
	}
	s := service.NewService(service.ServiceConfig{
		DryRun: globalFlags.dryRun,
	}, service.ServiceDeps{
		Logger: log,
		Fleet:  fleet,
	})
	if err := s.Run(); err != nil {
		Exitf("Error: %#v", err)
	}
}
