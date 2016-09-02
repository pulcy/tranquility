package service

import (
	"sync"

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
	invalidUnits := make(chan fleet.FleetUnitState)
	var ae errors.AggregateError
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.Fleet.InvalidUnits(invalidUnits); err != nil {
			ae.Add(err)
		}
		close(invalidUnits)
	}()

	fix := !s.DryRun
	for unitState := range invalidUnits {
		s.Logger.Infof("Found invalid unit: %s", unitState.String())
		if fix {
			if err := s.tryFixUnit(unitState); err != nil {
				s.Logger.Errorf("Failed to fix unit %s", unitState.Name)
			}
		}
	}

	wg.Wait()

	return maskAny(ae.AsError())
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
