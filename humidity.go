// Copyright (C) 2021, Benjamin Drung <bdrung@posteo.de>
// SPDX-License-Identifier: ISC

package main

import "math"

const (
	R       = 8.31446261815324 // molar gas constant R in kg * m² / (s² * K * mol)
	M_water = 0.01801528       // molar mass of water M(H2O) in kg / mol
	R_water = R / M_water      // specific gas constant for water vapor in m² / (s² * K)
)

// saturationVaporPressureWater calculates the saturation vapour pressure of water in hectopascal
// (hPa) with Arden Buck equation, because it is the most accurate formula for room temperatures.
// See https://en.wikipedia.org/wiki/Vapour_pressure_of_water#Accuracy_of_different_formulations
func saturationVaporPressureWater(tempCelsius float64) float64 {
	return 6.1121 * math.Exp((18.678-tempCelsius/234.5)*(tempCelsius/(257.14+tempCelsius)))
}

// Relative2AbsoluteHumidity calculates the absolute humidity in g/m³ for a given
// relative humidity (rh) and temperature in Celsius.
//
// The humidity definitions and the ideal gas law were used for deriving the formula:
// 1. AH = m_water / V
// 2. RH = p_water / p*_water
// 3. p_water = (m_water / V) * R_water * T
//
// Resulting formula: AH = RH * p*_water / (R_water * T)
//
// Symbols:
// AH: absolute humidity
// m_water: mass of the water vapor
// p_water: partial vapor pressure of water
// p*_water: saturation vapour pressure of water
// R_water: specific gas constant for water vapor
// RH: relative humidity
// T: temperature (in Kelvin)
// V: volume of the air and water vapor mixture
func Relative2AbsoluteHumidity(rh float64, tempCelsius float64) float64 {
	tempK := tempCelsius + 273.15
	return 1000 * rh * saturationVaporPressureWater(tempCelsius) / (R_water * tempK)
}
