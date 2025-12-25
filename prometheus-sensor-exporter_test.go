// Copyright (C) 2021-2025, Benjamin Drung <bdrung@posteo.de>
// SPDX-License-Identifier: ISC

package main

import (
	"reflect"
	"strings"
	"testing"
)

func intptr(v int) *int {
	return &v
}

func uint8ptr(v uint8) *uint8 {
	return &v
}

func TestParseSensorFlags(t *testing.T) {
	flags, err := parseSensorFlags(
		"SHT35,bus=1,address=0x45,repeatability=high,temp_offset=-0.5,humidity_offset=2.5")
	if err != nil {
		t.Errorf("Failed to parse flags: %s", err)
	}
	if flags.String() != "SHT35,address=0x45,bus=1,repeatability=high,temp_offset=-0.5,humidity_offset=2.5" {
		t.Errorf("String representation is incorrect: %s", flags)
	}
}

func TestParseSensorFlagsFailure(t *testing.T) {
	tests := []struct {
		name      string
		sensor    string
		wantedErr string
	}{
		{"model", "SHT35,foo=bar", "Unknown sensor option 'foo'."},
		{"address", "SHT35,address=-42", "Specified address '-42' is not an unsigned integer: "},
		{"bus", "SHT35,bus=foo", "Specified bus 'foo' is not an integer: "},
		{"temp_offset", "SHT35,temp_offset=caffee", "Failed to parse temperature offset 'caffee': "},
		{"humidity_offset", "SHT35,humidity_offset=hum", "Failed to parse humidity offset 'hum': "},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSensorFlags(test.sensor)
			if err == nil || !strings.Contains(err.Error(), test.wantedErr) {
				t.Errorf(
					"Incorrect error for sensor '%s', got: %v, want: %s.",
					test.sensor, err, test.wantedErr)
			}
		})
	}
}

func TestParseSensors(t *testing.T) {
	args := []string{"SHT35,bus=1,address=0x46", "BME280,bus=0"}
	want := []SensorFlags{
		{Model: "SHT35", Address: uint8ptr(0x46), Bus: intptr(1)},
		{Model: "BME280", Bus: intptr(0)},
	}

	got, err := parseSensors(args)
	if err != nil {
		t.Fatalf("parseSensors() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseSensors() = %v, want %v", got, want)
	}
}

func TestParseSensorsInvalid(t *testing.T) {
	args := []string{"SHT31,badflag"}
	wantedErr := "sensor 1 'SHT31,badflag': Unknown sensor option 'badflag'"

	_, err := parseSensors(args)
	if err == nil || !strings.Contains(err.Error(), wantedErr) {
		t.Fatalf("parseSensors() expected error '%s', got %v", wantedErr, err)
	}
}

func TestRound64(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		precision int
		want      float64
	}{
		{"round up", 3.14159, 2, 3.14},
		{"round down", 2.71828, 2, 2.72},
		{"zero precision", 2.71828, 0, 3},
		{"negative number", -1.2345, 2, -1.23},
		{"no rounding needed", 5.0, 2, 5.0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := round64(test.value, test.precision)
			if got != test.want {
				t.Errorf("round64(%v, %d) = %v, want %v", test.value, test.precision, got, test.want)
			}
		})
	}
}
