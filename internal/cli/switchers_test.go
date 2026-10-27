// SPDX-FileCopyrightText: 2026 Vedran Lebo <vedran@flyingpenguin.tech>
// SPDX-License-Identifier: MIT

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vlebo/ctx/internal/config"
)

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~/test", filepath.Join(home, "test")},
		{"~/.ssh/id_rsa", filepath.Join(home, ".ssh", "id_rsa")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := expandPath(tt.input)
			if result != tt.expected {
				t.Errorf("expandPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSwitchVPN_NilConfig(t *testing.T) {
	err := switchVPN(nil, nil)
	if err != nil {
		t.Errorf("switchVPN(nil, nil) = %v, want nil", err)
	}
}

func TestSwitchVPN_UnsupportedType(t *testing.T) {
	cfg := &config.VPNConfig{
		Type: "unsupported",
	}

	err := switchVPN(cfg, nil)
	if err == nil {
		t.Error("Expected error for unsupported VPN type")
	}
}

func TestSwitchVPN_CustomWithoutCmd(t *testing.T) {
	cfg := &config.VPNConfig{
		Type: config.VPNTypeCustom,
	}

	err := switchVPN(cfg, nil)
	if err == nil {
		t.Error("Expected error for custom VPN without connect_cmd")
	}
}

func TestSwitchVault_NilConfig(t *testing.T) {
	err := switchVault(nil, nil, nil, "")
	if err != nil {
		t.Errorf("switchVault(nil) = %v, want nil", err)
	}
}

func TestSwitchGit_NilConfig(t *testing.T) {
	err := switchGit(nil)
	if err != nil {
		t.Errorf("switchGit(nil) = %v, want nil", err)
	}
}

func TestSwitchDocker_NilConfig(t *testing.T) {
	err := switchDocker(nil)
	if err != nil {
		t.Errorf("switchDocker(nil) = %v, want nil", err)
	}
}

func TestSwitchNPM_NilConfig(t *testing.T) {
	err := switchNPM(nil)
	if err != nil {
		t.Errorf("switchNPM(nil) = %v, want nil", err)
	}
}

func TestDisconnectVPN_NilConfig(t *testing.T) {
	err := disconnectVPN(nil)
	if err != nil {
		t.Errorf("disconnectVPN(nil) = %v, want nil", err)
	}
}

func TestSwitchOpenVPN_NoConfigFile(t *testing.T) {
	cfg := &config.VPNConfig{
		Type: config.VPNTypeOpenVPN,
	}

	err := switchOpenVPN(cfg)
	if err == nil {
		t.Error("Expected error when config_file is not set")
	}
}

func TestSwitchNetbird_NoBinary(t *testing.T) {
	// Ensure netbird is not in PATH
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", origPath)

	cfg := &config.VPNConfig{
		Type: config.VPNTypeNetbird,
	}

	err := switchNetbird(cfg, nil)
	if err == nil {
		t.Error("Expected error when netbird is not in PATH")
	}
}

func TestSwitchWireGuard_NoInterface(t *testing.T) {
	cfg := &config.VPNConfig{
		Type: config.VPNTypeWireGuard,
	}

	err := switchWireGuard(cfg)
	if err == nil {
		t.Error("Expected error when interface is not set")
	}
}

// fakeBinDir puts stub executables that exit with the given codes on PATH.
func fakeBinDir(t *testing.T, exitCodes map[string]int) {
	t.Helper()
	dir := t.TempDir()
	for name, code := range exitCodes {
		script := fmt.Sprintf("#!/bin/sh\nexit %d\n", code)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func TestSwitchKubernetes_ContextSwitched(t *testing.T) {
	fakeBinDir(t, map[string]int{"kubectl": 0})
	mgr := config.NewManagerWithDir(t.TempDir())
	ctx := &config.ContextConfig{Name: "test"}
	cfg := &config.KubernetesConfig{Context: "cluster3"}

	if err := switchKubernetes(cfg, ctx, mgr); err != nil {
		t.Errorf("switchKubernetes() error = %v, want nil", err)
	}
}

func TestSwitchKubernetes_ContextNotFound(t *testing.T) {
	fakeBinDir(t, map[string]int{"kubectl": 1})
	mgr := config.NewManagerWithDir(t.TempDir())
	ctx := &config.ContextConfig{Name: "test"}
	cfg := &config.KubernetesConfig{Context: "cluster3"}

	if err := switchKubernetes(cfg, ctx, mgr); err == nil {
		t.Error("switchKubernetes() error = nil, want error when kubectl context is missing")
	}
}

func TestSwitchKubernetes_EKSFetchFailed(t *testing.T) {
	fakeBinDir(t, map[string]int{"kubectl": 0, "aws": 255})
	mgr := config.NewManagerWithDir(t.TempDir())
	ctx := &config.ContextConfig{
		Name: "test",
		AWS:  &config.AWSConfig{Profile: "bwstaging", Region: "eu-west-1"},
	}
	cfg := &config.KubernetesConfig{
		Context: "cluster3",
		EKS:     &config.EKSConfig{Cluster: "cluster3"},
	}

	if err := switchKubernetes(cfg, ctx, mgr); err == nil {
		t.Error("switchKubernetes() error = nil, want error when EKS credential fetch fails")
	}
}
