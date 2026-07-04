// SPDX-FileCopyrightText: 2026 Vedran Lebo <vedran@flyingpenguin.tech>
// SPDX-License-Identifier: MIT

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vlebo/ctx/internal/config"
)

func TestCheckSSMDependencies_MissingAWS(t *testing.T) {
	// Put an empty PATH so neither aws nor session-manager-plugin is found
	tmpDir := t.TempDir()
	t.Setenv("PATH", tmpDir)

	err := checkSSMDependencies()
	if err == nil {
		t.Fatal("expected error when aws CLI is missing, got nil")
	}
}

func TestCheckSSMDependencies_MissingPlugin(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake aws binary
	awsBin := filepath.Join(tmpDir, "aws")
	if err := os.WriteFile(awsBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpDir)

	err := checkSSMDependencies()
	if err == nil {
		t.Fatal("expected error when session-manager-plugin is missing, got nil")
	}
}

func TestCheckSSMDependencies_BothPresent(t *testing.T) {
	tmpDir := t.TempDir()

	for _, bin := range []string{"aws", "session-manager-plugin"} {
		p := filepath.Join(tmpDir, bin)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("PATH", tmpDir)

	if err := checkSSMDependencies(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestBuildSSMArgs(t *testing.T) {
	tunnel := config.TunnelConfig{
		Name:       "postgres",
		Type:       "aws",
		Target:     config.TunnelTarget{Kind: config.TunnelTargetKindLiteral, Literal: "i-0abc123def456789a"},
		RemoteHost: "db.internal.vpc",
		RemotePort: 5432,
		LocalPort:  5432,
	}

	args := buildSSMArgs("i-0abc123def456789a", tunnel)

	expected := []string{
		"ssm", "start-session",
		"--target", "i-0abc123def456789a",
		"--document-name", "AWS-StartPortForwardingSessionToRemoteHost",
		"--parameters", `{"host":["db.internal.vpc"],"portNumber":["5432"],"localPortNumber":["5432"]}`,
	}

	if len(args) != len(expected) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(expected), args)
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("arg[%d]: got %q, want %q", i, a, expected[i])
		}
	}
}

func TestBuildSSMArgs_PortSubstitution(t *testing.T) {
	tunnel := config.TunnelConfig{
		Type:       "aws",
		Target:     config.TunnelTarget{Kind: config.TunnelTargetKindLiteral, Literal: "i-0abc123def456789a"},
		RemoteHost: "cache.internal",
		RemotePort: 6379,
		LocalPort:  16379, // remapped port
	}

	args := buildSSMArgs("i-0abc123def456789a", tunnel)

	// Find --parameters arg
	var params string
	for i, a := range args {
		if a == "--parameters" && i+1 < len(args) {
			params = args[i+1]
			break
		}
	}

	want := `{"host":["cache.internal"],"portNumber":["6379"],"localPortNumber":["16379"]}`
	if params != want {
		t.Errorf("params: got %q, want %q", params, want)
	}
}

func TestValidateSSMTarget_Found(t *testing.T) {
	tmpDir := t.TempDir()

	// Fake aws binary that returns a non-empty InstanceInformationList
	response := map[string]any{
		"InstanceInformationList": []map[string]string{
			{"InstanceId": "i-0abc123def456789a"},
		},
	}
	out, _ := json.Marshal(response)
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", string(out))
	if err := os.WriteFile(filepath.Join(tmpDir, "aws"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpDir)
	awsEnv := append(os.Environ(), "PATH="+tmpDir)

	if err := validateSSMTarget("i-0abc123def456789a", awsEnv); err != nil {
		t.Fatalf("expected no error for registered instance, got: %v", err)
	}
}

func TestValidateSSMTarget_NotRegistered(t *testing.T) {
	tmpDir := t.TempDir()

	// Fake aws binary that returns an empty list
	response := map[string]any{
		"InstanceInformationList": []any{},
	}
	out, _ := json.Marshal(response)
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", string(out))
	if err := os.WriteFile(filepath.Join(tmpDir, "aws"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmpDir)
	awsEnv := append(os.Environ(), "PATH="+tmpDir)

	err := validateSSMTarget("i-0deadbeef00000000", awsEnv)
	if err == nil {
		t.Fatal("expected error for unregistered instance, got nil")
	}
}

func TestBuildAWSEnv_WithVaultCreds(t *testing.T) {
	awsCfg := &config.AWSConfig{
		Region:  "eu-west-1",
		Profile: "my-profile",
	}
	creds := &config.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "token123",
	}

	env := buildAWSEnv(awsCfg, creds)

	check := func(key, want string) {
		t.Helper()
		for _, e := range env {
			if len(e) > len(key)+1 && e[:len(key)+1] == key+"=" {
				got := e[len(key)+1:]
				if got != want {
					t.Errorf("%s: got %q, want %q", key, got, want)
				}
				return
			}
		}
		t.Errorf("missing env var %s", key)
	}

	check("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	check("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	check("AWS_SESSION_TOKEN", "token123")
	check("AWS_REGION", "eu-west-1")
	check("AWS_DEFAULT_REGION", "eu-west-1")
}

func TestBuildAWSEnv_WithProfile(t *testing.T) {
	awsCfg := &config.AWSConfig{
		Region:  "us-east-1",
		Profile: "sso-prod",
	}

	env := buildAWSEnv(awsCfg, nil)

	hasProfile := false
	hasNoKey := true
	for _, e := range env {
		if e == "AWS_PROFILE=sso-prod" {
			hasProfile = true
		}
		if len(e) > 17 && e[:17] == "AWS_ACCESS_KEY_ID" {
			hasNoKey = false
		}
	}
	if !hasProfile {
		t.Error("AWS_PROFILE not set when no vault creds")
	}
	if !hasNoKey {
		t.Error("AWS_ACCESS_KEY_ID should not be set when using profile")
	}
}

func TestBuildAWSEnv_CustomConfigFile(t *testing.T) {
	awsCfg := &config.AWSConfig{
		Profile: "myproject",
		Region:  "eu-west-1",
		Config:  "/home/user/.aws/config_myproject",
	}

	env := buildAWSEnv(awsCfg, nil)

	hasConfigFile := false
	for _, e := range env {
		if e == "AWS_CONFIG_FILE=/home/user/.aws/config_myproject" {
			hasConfigFile = true
		}
	}
	if !hasConfigFile {
		t.Error("AWS_CONFIG_FILE not set when aws.config is configured")
	}
}

func TestBuildAWSEnv_HostVarsAreFiltered(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")
	t.Setenv("AWS_PROFILE", "host-profile")
	t.Setenv("AWS_ACCESS_KEY_ID", "HOST_KEY_ID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "HOST_SECRET")

	awsCfg := &config.AWSConfig{Region: "eu-west-1"}
	creds := &config.AWSCredentials{
		AccessKeyID:     "CTX_KEY_ID",
		SecretAccessKey: "CTX_SECRET",
	}

	env := buildAWSEnv(awsCfg, creds)

	counts := map[string]int{}
	values := map[string]string{}
	for _, e := range env {
		for _, key := range []string{"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_PROFILE", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"} {
			if strings.HasPrefix(e, key+"=") {
				counts[key]++
				if counts[key] == 1 {
					values[key] = e[len(key)+1:]
				}
			}
		}
	}

	for _, key := range []string{"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"} {
		if counts[key] != 1 {
			t.Errorf("%s: expected exactly 1 occurrence, got %d", key, counts[key])
		}
	}
	if values["AWS_REGION"] != "eu-west-1" {
		t.Errorf("AWS_REGION: got %q, want %q", values["AWS_REGION"], "eu-west-1")
	}
	if values["AWS_ACCESS_KEY_ID"] != "CTX_KEY_ID" {
		t.Errorf("AWS_ACCESS_KEY_ID: got %q, want %q", values["AWS_ACCESS_KEY_ID"], "CTX_KEY_ID")
	}
	if values["AWS_SECRET_ACCESS_KEY"] != "CTX_SECRET" {
		t.Errorf("AWS_SECRET_ACCESS_KEY: got %q, want %q", values["AWS_SECRET_ACCESS_KEY"], "CTX_SECRET")
	}
	if counts["AWS_PROFILE"] != 0 {
		t.Errorf("AWS_PROFILE should not appear when vault creds are used, got %d occurrences", counts["AWS_PROFILE"])
	}
}

// --- resolveAWSTarget tests ---

// fakeAWSBin writes a shell script at <tmpDir>/aws that prints the given JSON output and exits 0.
func fakeAWSBin(t *testing.T, tmpDir, responseJSON string) []string {
	t.Helper()
	script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", responseJSON)
	if err := os.WriteFile(filepath.Join(tmpDir, "aws"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return append(os.Environ(), "PATH="+tmpDir)
}

// fakeAWSBinFail writes a shell script at <tmpDir>/aws that exits non-zero.
func fakeAWSBinFail(t *testing.T, tmpDir string) []string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(tmpDir, "aws"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return append(os.Environ(), "PATH="+tmpDir)
}

func TestResolveAWSTarget_Literal(t *testing.T) {
	target := config.TunnelTarget{Kind: config.TunnelTargetKindLiteral, Literal: "i-0abc123def456789a"}
	got, err := resolveAWSTarget(target, os.Environ())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "i-0abc123def456789a" {
		t.Errorf("got %q, want %q", got, "i-0abc123def456789a")
	}
}

func TestResolveAWSTarget_Export_Found(t *testing.T) {
	tmpDir := t.TempDir()
	response := map[string]any{
		"Exports": []map[string]string{
			{"Name": "MyStack-BastionInstanceId", "Value": "i-0abc123def456789a"},
		},
	}
	out, _ := json.Marshal(response)
	awsEnv := fakeAWSBin(t, tmpDir, string(out))
	t.Setenv("PATH", tmpDir)

	target := config.TunnelTarget{Kind: config.TunnelTargetKindExport, Export: "MyStack-BastionInstanceId"}
	got, err := resolveAWSTarget(target, awsEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "i-0abc123def456789a" {
		t.Errorf("got %q, want %q", got, "i-0abc123def456789a")
	}
}

func TestResolveAWSTarget_Export_Paginated(t *testing.T) {
	tmpDir := t.TempDir()

	// First page returns no match but includes NextToken; second page (--next-token arg) has the export.
	// Uses only shell builtins (case/$*) so the script works even when PATH=tmpDir only.
	script := `#!/bin/sh
case "$*" in
  *--next-token*)
    echo '{"Exports":[{"Name":"MyStack-BastionInstanceId","Value":"i-0abc123def456789a"}]}'
    ;;
  *)
    echo '{"Exports":[{"Name":"OtherExport","Value":"i-other"}],"NextToken":"page2"}'
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(tmpDir, "aws"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmpDir)
	awsEnv := os.Environ()

	target := config.TunnelTarget{Kind: config.TunnelTargetKindExport, Export: "MyStack-BastionInstanceId"}
	got, err := resolveAWSTarget(target, awsEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "i-0abc123def456789a" {
		t.Errorf("got %q, want %q", got, "i-0abc123def456789a")
	}
}

func TestResolveAWSTarget_Export_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	response := map[string]any{"Exports": []any{}}
	out, _ := json.Marshal(response)
	awsEnv := fakeAWSBin(t, tmpDir, string(out))
	t.Setenv("PATH", tmpDir)

	target := config.TunnelTarget{Kind: config.TunnelTargetKindExport, Export: "NonExistent-Export"}
	_, err := resolveAWSTarget(target, awsEnv)
	if err == nil {
		t.Fatal("expected error for missing export, got nil")
	}
}

func TestResolveAWSTarget_Export_AWSFails(t *testing.T) {
	tmpDir := t.TempDir()
	awsEnv := fakeAWSBinFail(t, tmpDir)
	t.Setenv("PATH", tmpDir)

	target := config.TunnelTarget{Kind: config.TunnelTargetKindExport, Export: "MyStack-BastionInstanceId"}
	_, err := resolveAWSTarget(target, awsEnv)
	if err == nil {
		t.Fatal("expected error when aws CLI fails, got nil")
	}
}

func TestResolveAWSTarget_StackOutput_Found(t *testing.T) {
	tmpDir := t.TempDir()
	response := map[string]any{
		"Stacks": []map[string]any{
			{
				"Outputs": []map[string]string{
					{"OutputKey": "BastionInstanceId", "OutputValue": "i-0abc123def456789a"},
				},
			},
		},
	}
	out, _ := json.Marshal(response)
	awsEnv := fakeAWSBin(t, tmpDir, string(out))
	t.Setenv("PATH", tmpDir)

	target := config.TunnelTarget{Kind: config.TunnelTargetKindStackOutput, Stack: "my-infra", Output: "BastionInstanceId"}
	got, err := resolveAWSTarget(target, awsEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "i-0abc123def456789a" {
		t.Errorf("got %q, want %q", got, "i-0abc123def456789a")
	}
}

func TestResolveAWSTarget_StackOutput_StackNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	awsEnv := fakeAWSBinFail(t, tmpDir)
	t.Setenv("PATH", tmpDir)

	target := config.TunnelTarget{Kind: config.TunnelTargetKindStackOutput, Stack: "nonexistent-stack", Output: "BastionInstanceId"}
	_, err := resolveAWSTarget(target, awsEnv)
	if err == nil {
		t.Fatal("expected error when stack not found, got nil")
	}
}

func TestResolveAWSTarget_StackOutput_OutputKeyNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	response := map[string]any{
		"Stacks": []map[string]any{
			{
				"Outputs": []map[string]string{
					{"OutputKey": "OtherKey", "OutputValue": "i-0abc123def456789a"},
				},
			},
		},
	}
	out, _ := json.Marshal(response)
	awsEnv := fakeAWSBin(t, tmpDir, string(out))
	t.Setenv("PATH", tmpDir)

	target := config.TunnelTarget{Kind: config.TunnelTargetKindStackOutput, Stack: "my-infra", Output: "BastionInstanceId"}
	_, err := resolveAWSTarget(target, awsEnv)
	if err == nil {
		t.Fatal("expected error for missing output key, got nil")
	}
}

func TestResolveAWSTarget_InvalidKind(t *testing.T) {
	target := config.TunnelTarget{Kind: config.TunnelTargetKindInvalid}
	_, err := resolveAWSTarget(target, os.Environ())
	if err == nil {
		t.Fatal("expected error for invalid target kind, got nil")
	}
}

// TestLoadTunnelState_OldTargetStringFormat reproduces the PR #24 review scenario:
// a state file written before TunnelConfig.Target became a struct, where Target
// was a plain string. TunnelConfig has no json tags of its own, so encoding/json
// falls back to matching Go field names case-insensitively (e.g. "Target").
func TestLoadTunnelState_OldTargetStringFormat(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "prod.json")
	oldFormat := `{
		"context_name": "prod",
		"tunnel_pids": {
			"bastion": {
				"started_at": "2026-01-01T00:00:00Z",
				"pid": 12345,
				"config": {
					"Name": "bastion",
					"Type": "aws",
					"Target": "i-0abc123def456789a",
					"RemoteHost": "127.0.0.1",
					"RemotePort": 5432,
					"LocalPort": 5432
				}
			}
		}
	}`
	if err := os.WriteFile(stateFile, []byte(oldFormat), 0o644); err != nil {
		t.Fatal(err)
	}

	state, err := loadTunnelState(stateFile)
	if err != nil {
		t.Fatalf("unexpected error loading old-format state file: %v", err)
	}

	entry, ok := state.TunnelPIDs["bastion"]
	if !ok {
		t.Fatal("expected bastion entry to survive unmarshal")
	}
	if entry.PID != 12345 {
		t.Errorf("PID = %d, want 12345", entry.PID)
	}
	target := entry.Config.Target
	if target.Kind != config.TunnelTargetKindLiteral {
		t.Errorf("Kind = %q, want %q", target.Kind, config.TunnelTargetKindLiteral)
	}
	if target.Literal != "i-0abc123def456789a" {
		t.Errorf("Literal = %q, want %q", target.Literal, "i-0abc123def456789a")
	}
}

// writeTunnelStateFile writes a tunnelState JSON file under stateDir/tunnels/<context>.json.
func writeTunnelStateFile(t *testing.T, stateDir, contextName string, state *tunnelState) string {
	t.Helper()
	tunnelDir := filepath.Join(stateDir, "tunnels")
	if err := os.MkdirAll(tunnelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(tunnelDir, contextName+".json")
	if err := saveTunnelState(stateFile, state); err != nil {
		t.Fatal(err)
	}
	return stateFile
}

// startSleepProcess starts a long-running sleep subprocess in its own process group
// (Setsid: true), mirroring how tunnels are launched. This is required so that
// killSSMProcessGroup (which uses Kill(-pid, SIGTERM)) actually reaches the process.
// Returns the PID and a reap function to call after killing to avoid zombies.
func startSleepProcess(t *testing.T) (pid int, reap func()) {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd.Process.Pid, func() { _ = cmd.Wait() }
}

func TestStopContextTunnels_StopsAWSTunnels(t *testing.T) {
	stateDir := t.TempDir()
	pid, reap := startSleepProcess(t)

	state := &tunnelState{
		TunnelPIDs: map[string]tunnelEntry{
			"postgres": {
				PID:       pid,
				StartedAt: time.Now(),
				Config: config.TunnelConfig{
					Name:       "postgres",
					Type:       "aws",
					Target:     config.TunnelTarget{Kind: config.TunnelTargetKindLiteral, Literal: "i-0abc123def456789a"},
					RemoteHost: "db.internal.vpc",
					RemotePort: 5432,
					LocalPort:  5432,
				},
			},
		},
	}
	stateFile := writeTunnelStateFile(t, stateDir, "test-ctx", state)

	stopped, err := stopContextTunnels(stateDir, "test-ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stopped != 1 {
		t.Errorf("expected 1 tunnel stopped, got %d", stopped)
	}

	// Reap the zombie so kill -0 no longer sees it
	reap()
	if isProcessRunning(pid) {
		t.Error("AWS tunnel process should have been stopped")
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Error("state file should have been removed")
	}
}

func TestStopContextTunnels_StopsBothSSHAndAWS(t *testing.T) {
	stateDir := t.TempDir()
	sshPID, reapSSH := startSleepProcess(t)
	awsPID, reapAWS := startSleepProcess(t)

	state := &tunnelState{
		TunnelPIDs: map[string]tunnelEntry{
			"legacy": {
				PID:       sshPID,
				StartedAt: time.Now(),
				Config:    config.TunnelConfig{Name: "legacy"},
			},
			"postgres": {
				PID:       awsPID,
				StartedAt: time.Now(),
				Config:    config.TunnelConfig{Name: "postgres", Type: "aws"},
			},
		},
	}
	stateFile := writeTunnelStateFile(t, stateDir, "test-ctx", state)

	stopped, err := stopContextTunnels(stateDir, "test-ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stopped != 2 {
		t.Errorf("expected 2 tunnels stopped, got %d", stopped)
	}

	reapSSH()
	reapAWS()
	if isProcessRunning(sshPID) {
		t.Error("SSH tunnel process should have been stopped")
	}
	if isProcessRunning(awsPID) {
		t.Error("AWS tunnel process should have been stopped")
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Error("state file should have been removed")
	}
}

func TestStopContextTunnels_NoStateFile(t *testing.T) {
	stateDir := t.TempDir()

	stopped, err := stopContextTunnels(stateDir, "nonexistent-ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stopped != 0 {
		t.Errorf("expected 0 tunnels stopped, got %d", stopped)
	}
}
