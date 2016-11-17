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

package fleet

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/coreos/fleet/client"
	"github.com/coreos/fleet/job"
	"github.com/coreos/fleet/schema"
	"github.com/op/go-logging"
)

const (
	maxStartDuration = time.Minute
	maxStopDuration  = time.Minute
)

type Fleet struct {
	api    client.API
	Logger *logging.Logger
}

type FleetUnitState struct {
	Name      string
	Failed    bool
	Inactive  bool
	Global    bool
	MachineID string
}

func (s FleetUnitState) String() string {
	return fmt.Sprintf("%s [%s]", s.Name, s.State())
}

func (s FleetUnitState) State() string {
	state := "?"
	if s.Failed {
		state = "failed"
	} else if s.Inactive {
		state = "inactive"
	}
	return state
}

// NewFleet creates a new fleet accessor
func NewFleet(endpoint url.URL, log *logging.Logger) (*Fleet, error) {
	api, err := createFleetClient(endpoint)
	if err != nil {
		return nil, maskAny(err)
	}
	return &Fleet{
		api:    api,
		Logger: log,
	}, nil
}

// Start instructs fleet to start the unit with the given name
func (f *Fleet) Start(name string) error {
	return maskAny(f.setUnitState(name, job.JobStateLaunched))
}

// WaitUntilStarted blocks until the unit with the given name is running, failed
// or a timeout occurs.
func (f *Fleet) WaitUntilStarted(name string) error {
	return maskAny(f.waitUntilStableState(name, job.JobStateLaunched, maxStopDuration))
}

// Stop instructs fleet to stop the unit with the given name
func (f *Fleet) Stop(name string) error {
	return maskAny(f.setUnitState(name, job.JobStateInactive))
}

// WaitUntilStopped blocks until the unit with the given name has stopped
// or a timeout occurs.
func (f *Fleet) WaitUntilStopped(name string) error {
	return maskAny(f.waitUntilStableState(name, job.JobStateInactive, maxStopDuration))
}

// waitUntilStableState blocks until the unit has reached the given unit state for a while
// or until a timeout occurs.
func (f *Fleet) waitUntilStableState(name string, unitState job.JobState, timeout time.Duration) error {
	start := time.Now()
	delay := time.Second * 2
	okCounter := 0
	for {
		time.Sleep(delay)
		ok, err := f.checkUnitState(name, unitState)
		if err != nil {
			return maskAny(err)
		}
		if ok {
			okCounter++
			delay = time.Second / 2
			if okCounter > 3 {
				return nil
			}
		} else {
			delay = time.Second * 2
			okCounter = 0
		}
		if time.Since(start) > timeout {
			return maskAny(TimeoutError)
		}
	}
}

func (f *Fleet) setUnitState(name string, unitState job.JobState) error {
	unit, err := f.unitWithRetry(name)
	if err != nil {
		return maskAny(err)
	}
	if unit.DesiredState == string(unitState) {
		f.Logger.Debugf("Unit %s already has desired state %s, doing nothing", name, unitState)
		return nil
	}
	operation := func() error {
		return maskAny(f.api.SetUnitTargetState(name, string(unitState)))
	}
	if err := backoff.Retry(operation, backoff.NewExponentialBackOff()); err != nil {
		return maskAny(err)
	}
	return nil
}

func (f *Fleet) checkUnitState(name string, unitState job.JobState) (bool, error) {
	// If this is a global unit, CurrentState will never be set. Instead, wait for DesiredState.
	unit, err := f.unitWithRetry(name)
	if err != nil {
		return false, maskAny(err)
	}
	var state string
	if suToGlobal(*unit) {
		state = unit.DesiredState
	} else {
		state = unit.CurrentState
	}
	f.Logger.Debugf("Unit %s state %s", name, state)
	return string(unitState) == state, nil
}

// InvalidUnits yields all failed or (incorrectly) invalid units in the given channel.
func (f *Fleet) InvalidUnits(ch chan<- FleetUnitState) error {
	var states []*schema.UnitState
	operation := func() error {
		var err error
		states, err = f.api.UnitStates()
		return maskAny(err)
	}
	if err := backoff.Retry(operation, backoff.NewExponentialBackOff()); err != nil {
		return maskAny(err)
	}

	for _, s := range states {
		switch s.SystemdActiveState {
		case "inactive":
			rc, err := f.shouldUnitBeActive(s.Name)
			if err == nil && rc {
				global, err := f.unitIsGlobal(s.Name)
				if err == nil {
					ch <- FleetUnitState{Name: s.Name, Inactive: true, Global: global, MachineID: s.MachineID}
				}
			}
		case "failed":
			global, err := f.unitIsGlobal(s.Name)
			if err == nil {
				ch <- FleetUnitState{Name: s.Name, Failed: true, Global: global, MachineID: s.MachineID}
			}
		}
	}

	return nil
}

// unitWithRetry wraps Unit of the fleet API with a retry.
func (f *Fleet) unitWithRetry(unitName string) (*schema.Unit, error) {
	var u *schema.Unit
	op := func() error {
		var err error
		u, err = f.api.Unit(unitName)
		return maskAny(err)
	}
	if err := backoff.Retry(op, backoff.NewExponentialBackOff()); err != nil {
		return u, maskAny(err)
	}
	return u, nil
}

func (f *Fleet) shouldUnitBeActive(name string) (bool, error) {
	if !strings.HasSuffix(name, ".service") {
		return false, nil
	}
	unit, err := f.unitWithRetry(name)
	if err != nil {
		return false, maskAny(err)
	}
	serviceType := ""
	stopWhenUnneeded := ""
	for _, option := range unit.Options {
		switch strings.ToLower(option.Section) {
		case "service":
			if strings.ToLower(option.Name) == "type" {
				serviceType = strings.ToLower(option.Value)
			}
		case "unit":
			if strings.ToLower(option.Name) == "stopwhenunneeded" {
				stopWhenUnneeded = strings.ToLower(option.Value)
			}
		}
	}
	if serviceType == "oneshot" {
		return false, nil
	}
	if stopWhenUnneeded == "yes" {
		return false, nil
	}
	return true, nil
}

func (f *Fleet) unitIsGlobal(name string) (bool, error) {
	unit, err := f.unitWithRetry(name)
	if err != nil {
		return false, maskAny(err)
	}
	return suToGlobal(*unit), nil
}

func createFleetClient(endpoint url.URL) (client.API, error) {
	var trans http.RoundTripper

	switch endpoint.Scheme {
	case "unix", "file":
		if len(endpoint.Host) > 0 {
			return nil, fmt.Errorf("unable to connect to host %q with scheme %q", endpoint.Host, endpoint.Scheme)
		}

		// The Path field is only used for dialing and should not be used when
		// building any further HTTP requests.
		sockPath := endpoint.Path
		endpoint.Path = ""

		// http.Client doesn't support the schemes "unix" or "file", but it
		// is safe to use "http" as dialFunc ignores it anyway.
		endpoint.Scheme = "http"

		// The Host field is not used for dialing, but will be exposed in debug logs.
		endpoint.Host = "domain-sock"

		trans = &http.Transport{
			Dial: func(s, t string) (net.Conn, error) {
				// http.Client does not natively support dialing a unix domain socket, so the
				// dial function must be overridden.
				return net.Dial("unix", sockPath)
			},
		}
	case "http", "https":
		trans = http.DefaultTransport
	default:
		return nil, fmt.Errorf("Unknown scheme in fleet endpoint: %s", endpoint.Scheme)
	}

	c := &http.Client{
		Transport: trans,
	}
	fAPI, err := client.NewHTTPClient(c, endpoint)
	if err != nil {
		return nil, maskAny(err)
	}
	return fAPI, nil
}

// suToGlobal returns whether or not a schema.Unit refers to a global unit
func suToGlobal(su schema.Unit) bool {
	u := job.Unit{
		Unit: *schema.MapSchemaUnitOptionsToUnitFile(su.Options),
	}
	return u.IsGlobal()
}
