// Copyright (C) 2021, Benjamin Drung <bdrung@posteo.de>
// SPDX-License-Identifier: ISC

package main

import "math"

const (
	gasConstant      = 8.31446261815324             // molar gas constant R in kg * m² / (s² * K * mol)
	molarMassWater   = 0.01801528                   // molar mass of water M(H2O) in kg / mol
	gasConstantWater = gasConstant / molarMassWater // specific gas constant for water vapor in m² / (s² * K)
)

// saturationVaporPressureWater calculates the saturation vapour pressure of water in hectopascal
// (hPa) with Arden Buck equation, because it is the most accurate formula for room temperatures.
// See https://en.wikipedia.org/wiki/Vapour_pressure_of_water#Accuracy_of_different_formulations
func saturationVaporPressureWater(temperatureCelsius float64) float64 {
	e := (18.678 - temperatureCelsius/234.5) * (temperatureCelsius / (257.14 + temperatureCelsius))
	return 6.1121 * math.Exp(e)
}

// Relative2AbsoluteHumidity calculates the absolute humidity in g/m³ for a given
// relative humidity and temperature in Celsius.
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
func Relative2AbsoluteHumidity(relativeHumidity float64, temperatureCelsius float64) float64 {
	temperatureKelvin := temperatureCelsius + 273.15
	return 1000 * relativeHumidity * saturationVaporPressureWater(temperatureCelsius) / (gasConstantWater * temperatureKelvin)
}
