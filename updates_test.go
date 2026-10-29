package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

func TestUpdateCommands(t *testing.T) {
	for _, test := range []struct {
		name, arg                                    string
		enabled, available, checkError, installError bool
		code, checks, installs                       int
		category                                     string
	}{
		{name: "version", arg: "--version"},
		{name: "disabled", arg: "--check-for-updates", code: 1, category: "updates_disabled"},
		{name: "current", arg: "--check-for-updates", enabled: true, checks: 1},
		{name: "available", arg: "--check-for-updates", enabled: true, available: true, checks: 1},
		{name: "check_failure", arg: "--check-for-updates", enabled: true, checkError: true, checks: 1, code: 1, category: "update_check_failed"},
		{name: "install", arg: "--install-update", enabled: true, available: true, checks: 1, installs: 1},
		{name: "install_failure", arg: "--install-update", enabled: true, available: true, installError: true, checks: 1, installs: 1, code: 1, category: "update_install_failed"},
		{name: "no_install_when_current", arg: "--install-update", enabled: true, checks: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			checks, installs := 0, 0
			client := updateCommands{current: "0.5.0-rc.1", enabled: func() bool { return test.enabled },
				check: func(ctx context.Context) (*mygo.Update, error) {
					checks++
					if _, ok := ctx.Deadline(); !ok {
						t.Fatal("unbounded check")
					}
					if test.checkError {
						return nil, errors.New("private fixture diagnostic")
					}
					if test.available {
						return &mygo.Update{Version: "0.5.0"}, nil
					}
					return nil, nil
				}, install: func(ctx context.Context, update *mygo.Update) error {
					installs++
					if _, ok := ctx.Deadline(); !ok {
						t.Fatal("unbounded install")
					}
					if update.Version != "0.5.0" {
						t.Fatal("wrong update")
					}
					if test.installError {
						return errors.New("private fixture diagnostic")
					}
					return nil
				}}
			handled, code := handleUpdateCommand([]string{test.arg}, &out, client)
			if !handled || code != test.code || checks != test.checks || installs != test.installs {
				t.Fatalf("unexpected command result: %v %d checks=%d installs=%d", handled, code, checks, installs)
			}
			if strings.Contains(out.String(), "private fixture") {
				t.Fatal("raw diagnostic leaked")
			}
			if test.arg == "--version" {
				if out.String() != "0.5.0-rc.1\n" {
					t.Fatal("wrong version")
				}
				return
			}
			var result map[string]any
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if test.category != "" && result["error"] != test.category {
				t.Fatal("missing error category")
			}
			if test.available && !test.checkError && result["available_version"] != "0.5.0" {
				t.Fatal("wrong available version")
			}
			if test.installs == 1 && !test.installError && result["install_staged"] != true {
				t.Fatal("staging not reported")
			}
		})
	}
}

func TestUpdateCommandsDoNotInterceptNormalLaunch(t *testing.T) {
	for _, args := range [][]string{nil, {"--unrelated"}, {"-psn_0_1"}} {
		if handled, _ := handleUpdateCommand(args, &bytes.Buffer{}, updateCommands{}); handled {
			t.Fatal("normal launch intercepted")
		}
	}
	var out bytes.Buffer
	if handled, code := handleUpdateCommand([]string{"--install-update", "extra"}, &out, updateCommands{}); !handled || code != 1 {
		t.Fatal("accepted extra arguments")
	}
}
