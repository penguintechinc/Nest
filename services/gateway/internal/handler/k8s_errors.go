package handler

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func isK8sNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}

func isK8sAlreadyExists(err error) bool {
	return apierrors.IsAlreadyExists(err)
}
