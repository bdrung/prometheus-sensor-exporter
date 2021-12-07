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
// 1. absoluteHumidity = massWaterVapor / VolumeAirAndWater
// 2. relativehumidity = partialVaporPressureWater / saturationVaporPressureWater
// 3. partialVaporPressureWater = (massWaterVapor / VolumeAirAndWater) * gasConstantWater * temperatureKelvin
//
// Resulting formula:
// absoluteHumidity = relativehumidity * saturationVaporPressureWater / (gasConstantWater * temperatureKelvin)
func Relative2AbsoluteHumidity(relativeHumidity float64, temperatureCelsius float64) float64 {
	temperatureKelvin := temperatureCelsius + 273.15
	return 1000 * relativeHumidity * saturationVaporPressureWater(temperatureCelsius) / (gasConstantWater * temperatureKelvin)
}
