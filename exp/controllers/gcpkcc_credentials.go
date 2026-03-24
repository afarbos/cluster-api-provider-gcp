/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	"golang.org/x/oauth2/google"
)

const gkeScope = "https://www.googleapis.com/auth/cloud-platform"

// generateGCPAccessToken returns a short-lived OAuth2 access token using
// Application Default Credentials (configured via GOOGLE_APPLICATION_CREDENTIALS
// on the controller pod).
func generateGCPAccessToken(ctx context.Context) (string, error) {
	ts, err := google.DefaultTokenSource(ctx, gkeScope)
	if err != nil {
		return "", fmt.Errorf("creating token source: %w", err)
	}
	token, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("generating access token: %w", err)
	}
	return token.AccessToken, nil
}
