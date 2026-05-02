package service

import "go.uber.org/zap"

type QueryService struct {
	logger *zap.Logger
}

func NewQueryService(logger *zap.Logger) *QueryService {
	return &QueryService{logger: logger}
}
