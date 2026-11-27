//go:build !windows

package cpa

import (
	"errors"
)

type authDoc struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt"`
	Endpoints    struct {
		APIOrigin string `json:"apiOrigin"`
	} `json:"endpoints"`
}

func readAuth(dir string) (*authDoc, error) {
	return nil, errors.New("local QwenWork client import is only available on Windows; use OAuth login instead")
}
