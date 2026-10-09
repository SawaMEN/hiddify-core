package hcore

import (
	"context"

	"github.com/hiddify/hiddify-core/v2/hcommon"
	"github.com/hiddify/hiddify-core/v2/service_manager"
)

var (
	sWorkingPath          string
	sTempPath             string
	sUserID               int
	sGroupID              int
	statusPropagationPort int64
)

func InitHiddifyService() error {
	return service_manager.StartServices()
}

func (s *CoreService) Setup(ctx context.Context, req *SetupRequest) (*hcommon.Response, error) {
	err := Setup(req, nil)
	code := hcommon.ResponseCode_OK
	if err != nil {
		return &hcommon.Response{Code: hcommon.ResponseCode_FAILED, Message: err.Error()}, err
	}
	return &hcommon.Response{Code: code}, nil
}
