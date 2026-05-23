package service

import "go.uber.org/zap"

type ObjectService struct {
	logger *zap.Logger
}

func NewObjectService(logger *zap.Logger) *ObjectService {
	return &ObjectService{logger: logger}
}
