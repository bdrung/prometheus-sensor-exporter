package main

import (
	"testing"
)

func TestSensorFlags(t *testing.T) {
	flags, err := parseSensorFlags("SHT35,bus=1,address=0x45")
	if err != nil {
		t.Errorf("Failed to parse flags: %s", err)
	}
	if flags.String() != "SHT35,address=0x45,bus=1" {
		t.Errorf("String representation is incorrect: %s", flags)
	}
}
