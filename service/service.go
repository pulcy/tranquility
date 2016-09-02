package service

import (
	"sync"
	"time"

	"github.com/ewoutp/go-aggregate-error"
	"github.com/op/go-logging"

	"github.com/pulcy/tranquility/fleet"
)

type Service struct {
	ServiceConfig
	ServiceDeps
}

type ServiceConfig struct {
	DryRun bool
}

type ServiceDeps struct {
	Logger *logging.Logger
	Fleet  *fleet.Fleet
}

// NewService instantiates a Service.
func NewService(config ServiceConfig, deps ServiceDeps) *Service {
	return &Service{
		ServiceConfig: config,
		ServiceDeps:   deps,
	}
}

// Run queries all invalid units and tries to get them in the correct state
func (s *Service) Run() error {
	initialInvalidUnits, err := s.collectInvalidUnits()
	if err != nil {
		return maskAny(err)
	}
	if len(initialInvalidUnits) == 0 {
		// No invalud units found
		return nil
	}

	s.Logger.Infof("Found %d invalid units, wait a bit for stability", len(initialInvalidUnits))
	time.Sleep(time.Second * 30)

	// Fetch invalid units again
	invalidUnitsChan := make(chan fleet.FleetUnitState)
	var ae errors.AggregateError
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.Fleet.InvalidUnits(invalidUnitsChan); err != nil {
			ae.Add(err)
		}
		close(invalidUnitsChan)
	}()

	// Start 5 processors to fix units
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fix := !s.DryRun
			for unitState := range invalidUnitsChan {
				_, stable := initialInvalidUnits[unitState.Name]
				if stable {
					s.Logger.Infof("Found invalid unit: %s", unitState.String())
					if fix {
						if err := s.tryFixUnit(unitState); err != nil {
							s.Logger.Errorf("Failed to fix unit %s", unitState.Name)
						}
					}
				} else {
					s.Logger.Infof("Found new invalid unit: %s", unitState.String())
				}
			}
		}()
	}

	wg.Wait()

	return maskAny(ae.AsError())
}

func (s *Service) collectInvalidUnits() (map[string]fleet.FleetUnitState, error) {
	// Collect invalid units
	invalidUnitsChan := make(chan fleet.FleetUnitState)
	invalidUnitsMap := make(map[string]fleet.FleetUnitState)
	var ae errors.AggregateError
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.Fleet.InvalidUnits(invalidUnitsChan); err != nil {
			ae.Add(err)
		}
		close(invalidUnitsChan)
	}()

	for unitState := range invalidUnitsChan {
		invalidUnitsMap[unitState.Name] = unitState
	}

	wg.Wait()

	return invalidUnitsMap, maskAny(ae.AsError())
}

func (s *Service) tryFixUnit(unit fleet.FleetUnitState) error {
	if !unit.Global {
		// Stop the unit first
		s.Logger.Infof("Stopping unit %s", unit.Name)
		s.Fleet.Stop(unit.Name)

		// Wait until stopped
		if err := s.Fleet.WaitUntilStopped(unit.Name); fleet.IsTimeout(err) {
			s.Logger.Errorf("Failed to stop unit %s in time", unit.Name)
		} else if err != nil {
			s.Logger.Errorf("Failed to stop unit %s: %#v", unit.Name, err)
		} else {
			s.Logger.Infof("Unit %s is stopped", unit.Name)
		}
	}

	// Now try to start it
	s.Logger.Infof("Starting unit %s", unit.Name)
	s.Fleet.Start(unit.Name)

	// Wait until started
	if err := s.Fleet.WaitUntilStarted(unit.Name); fleet.IsTimeout(err) {
		s.Logger.Errorf("Failed to start unit %s in time", unit.Name)
		return maskAny(err)
	} else if err != nil {
		s.Logger.Errorf("Failed to start unit %s: %#v", unit.Name, err)
		return maskAny(err)
	} else {
		s.Logger.Infof("Unit %s is started", unit.Name)
	}

	return nil
}
