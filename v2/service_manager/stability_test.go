package service_manager

import (
	"errors"
	"github.com/sagernet/sing-box/option"
	"testing"
)

type testService struct {
	disposed, closed int
	failure          error
}

func (*testService) Init() error                                 { return nil }
func (s *testService) Dispose() error                            { s.disposed++; return s.failure }
func (*testService) OnMainServicePreStart(*option.Options) error { return nil }
func (*testService) OnMainServiceStart() error                   { return nil }
func (s *testService) OnMainServiceClose() error                 { s.closed++; return s.failure }
func TestPreServicesAndCleanupAfterFailure(t *testing.T) {
	oldServices, oldPreServices := services, preservices
	defer func() { services, preservices = oldServices, oldPreServices }()
	services, preservices = nil, nil
	main := &testService{failure: errors.New("cleanup failed")}
	first, second := &testService{}, &testService{}
	Register(main)
	RegisterPreService(first)
	RegisterPreService(second)
	if len(preservices) != 2 || preservices[0] != first {
		t.Fatal("pre-services overwritten by main services")
	}
	if err := DisposeServices(); err == nil {
		t.Fatal("cleanup failure hidden")
	}
	if first.disposed != 1 || second.disposed != 1 {
		t.Fatal("cleanup stopped after first failure")
	}
	if err := OnMainServiceClose(); err == nil {
		t.Fatal("close failure hidden")
	}
	if first.closed != 1 || second.closed != 1 {
		t.Fatal("close skipped services")
	}
}
