package service

import "go.uber.org/zap"

type VolumeService struct {
	logger *zap.Logger
}

func NewVolumeService(logger *zap.Logger) *VolumeService {
	return &VolumeService{logger: logger}
}
