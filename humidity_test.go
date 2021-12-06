// Copyright (C) 2021, Benjamin Drung <bdrung@posteo.de>
// SPDX-License-Identifier: ISC

package main

import (
	"math"
	"testing"
)

func TestRelative2AbsoluteHumidity(t *testing.T) {
	tests := []struct {
		rh          float64
		tempCelsius float64
		ah          float64
	}{
		{40.0, 20.0, 6.9},
		{50.0, 15.0, 6.4},
		{70.0, 20.0, 12.1},
		{80.0, 15.0, 10.3},
		{80.0, -10.0, 1.9},
		{20.0, 50.0, 16.6},
	}

	for _, test := range tests {
		ah := Relative2AbsoluteHumidity(test.rh, test.tempCelsius)
		if math.Abs(ah-test.ah) > 0.05 {
			t.Errorf(
				"Absolute humidity for %f%% humidity at %f° C was incorrect, got: %f, want: %f.",
				test.rh, test.tempCelsius, ah, test.ah)
		}
	}
}
