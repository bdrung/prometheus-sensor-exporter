// Copyright (C) 2021-2025, Benjamin Drung <bdrung@posteo.de>
// SPDX-License-Identifier: ISC

package main

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"

	bsbmp "github.com/d2r2/go-bsbmp"
	i2c "github.com/d2r2/go-i2c"
	logger "github.com/d2r2/go-logger"
	sht3x "github.com/d2r2/go-sht3x"
	"github.com/prometheus/client_golang/prometheus"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

type Readings struct {
	temperature *float64
	humidity    *float64
}

type Sensor interface {
	Poll() (Readings, error)
	Labels() prometheus.Labels
}

type BMPSensor struct {
	Address uint8
	Bus     int
	Model   string
	bmp     *bsbmp.BMP
	mutex   sync.Mutex
}

func NewBMPSensor(
	address uint8,
	bus int,
	model string,
	sensorType bsbmp.SensorType,
) (*BMPSensor, error) {
	logrus.Infof("New BMP sensor: %s,address=0x%x,bus=%d", model, address, bus)
	i2c, err := i2c.NewI2C(address, bus)
	if err != nil {
		return nil, err
	}
	bmp, err := bsbmp.NewBMP(sensorType, i2c)
	if err != nil {
		return nil, err
	}
	return &BMPSensor{
		Address: address,
		Bus:     bus,
		Model:   model,
		bmp:     bmp,
	}, nil
}

func (s BMPSensor) Labels() prometheus.Labels {
	return prometheus.Labels{
		"address": fmt.Sprintf("0x%x", s.Address),
		"bus":     fmt.Sprintf("%d", s.Bus),
		"model":   s.Model,
	}
}

func (s BMPSensor) Poll() (Readings, error) {
	var readings Readings

	s.mutex.Lock()
	temp, err := s.bmp.ReadTemperatureC(bsbmp.ACCURACY_STANDARD)
	s.mutex.Unlock()
	if err != nil {
		return readings, err
	}
	rounded_temp := round64(float64(temp), 2)
	readings.temperature = &rounded_temp

	// TODO: read temperature and humidity in one go for BME280
	s.mutex.Lock()
	supported, rh, err := s.bmp.ReadHumidityRH(bsbmp.ACCURACY_STANDARD)
	s.mutex.Unlock()
	if err != nil {
		return readings, err
	}
	if supported {
		rounded_rh := round64(float64(rh), 2)
		readings.humidity = &rounded_rh
	}

	// TODO: Read pressure as well
	return readings, nil
}

type SHT3xSensor struct {
	Address           uint8
	Bus               int
	Model             string
	I2C               *i2c.I2C
	SHT3X             sht3x.SHT3X
	mutex             sync.Mutex
	repeatability     sht3x.MeasureRepeatability
	repeatability_str string
}

func NewSHT3xSensor(
	address uint8,
	bus int,
	model string,
	repeatability sht3x.MeasureRepeatability,
	repeatability_str string,
) (*SHT3xSensor, error) {
	logrus.Infof(
		"New SHT3x sensor: %s,address=0x%x,bus=%d,repeatability=%s",
		model,
		address,
		bus,
		repeatability_str,
	)
	i2c, err := i2c.NewI2C(address, bus)
	if err != nil {
		return nil, err
	}
	return &SHT3xSensor{
		Address:           address,
		Bus:               bus,
		Model:             model,
		I2C:               i2c,
		SHT3X:             *sht3x.NewSHT3X(),
		repeatability:     repeatability,
		repeatability_str: repeatability_str,
	}, nil
}

func (s SHT3xSensor) Labels() prometheus.Labels {
	return prometheus.Labels{
		"address":       fmt.Sprintf("0x%x", s.Address),
		"bus":           fmt.Sprintf("%d", s.Bus),
		"model":         s.Model,
		"repeatability": s.repeatability_str,
	}
}

func (s SHT3xSensor) Poll() (Readings, error) {
	var readings Readings

	s.mutex.Lock()
	temp, rh, err := s.SHT3X.ReadTemperatureAndRelativeHumidity(s.I2C, s.repeatability)
	s.mutex.Unlock()
	if err != nil {
		return readings, err
	}

	rounded_temp := round64(float64(temp), 2)
	rounded_rh := round64(float64(rh), 2)
	readings.temperature = &rounded_temp
	readings.humidity = &rounded_rh
	return readings, nil
}

type SensorFlags struct {
	Model          string
	Address        *uint8
	Bus            *int
	Repeatability  string
	TempOffset     float64
	HumidityOffset float64
}

func parseSensorFlags(sensor string) (SensorFlags, error) {
	var flags SensorFlags
	fields := strings.Split(sensor, ",")
	flags.Model = fields[0]
	for _, field := range fields[1:] {
		key_value := strings.SplitN(field, "=", 2)
		var value string
		if len(key_value) == 2 {
			value = key_value[1]
		}
		switch key_value[0] {
		case "address":
			if address8, err := strconv.ParseUint(value, 0, 8); err == nil {
				address := uint8(address8)
				flags.Address = &address
			} else {
				return flags,
					fmt.Errorf("Specified address '%s' is not an unsigned integer: %s", value, err)
			}
		case "bus":
			if bus32, err := strconv.ParseInt(value, 0, 32); err == nil {
				bus := int(bus32)
				flags.Bus = &bus
			} else {
				return flags, fmt.Errorf("Specified bus '%s' is not an integer: %s", value, err)
			}
		case "repeatability":
			flags.Repeatability = value
		case "temp_offset":
			var err error
			flags.TempOffset, err = strconv.ParseFloat(value, 64)
			if err != nil {
				return flags, fmt.Errorf("Failed to parse temperature offset '%s': %s", value, err)
			}
		case "humidity_offset":
			var err error
			flags.HumidityOffset, err = strconv.ParseFloat(value, 64)
			if err != nil {
				return flags, fmt.Errorf("Failed to parse humidity offset '%s': %s", value, err)
			}
		default:
			return flags, fmt.Errorf("Unknown sensor option '%s'.", key_value[0])
		}
	}
	return flags, nil
}

func (s SensorFlags) NewBMPSensor(sensorType bsbmp.SensorType) (*BMPSensor, error) {
	// Defaults
	if s.Address == nil {
		address := uint8(0x76)
		s.Address = &address
	}
	if s.Bus == nil {
		bus := 0
		s.Bus = &bus
	}

	return NewBMPSensor(*s.Address, *s.Bus, s.Model, sensorType)
}

func (s SensorFlags) NewSHT3xSensor() (*SHT3xSensor, error) {
	// Defaults
	if s.Address == nil {
		address := uint8(0x45)
		s.Address = &address
	}
	if s.Bus == nil {
		bus := 0
		s.Bus = &bus
	}
	if s.Repeatability == "" {
		s.Repeatability = "high"
	}

	var repeatability sht3x.MeasureRepeatability
	switch s.Repeatability {
	case "low":
		repeatability = sht3x.RepeatabilityLow
	case "medium":
		repeatability = sht3x.RepeatabilityMedium
	case "high":
		repeatability = sht3x.RepeatabilityHigh
	default:
		return nil, fmt.Errorf("Unknown repeatability: %s", s.Repeatability)
	}

	return NewSHT3xSensor(*s.Address, *s.Bus, s.Model, repeatability, s.Repeatability)
}

func (s SensorFlags) NewSensor() (Sensor, error) {
	switch s.Model {
	case "BME280":
		return s.NewBMPSensor(bsbmp.BME280)
	case "BMP180":
		return s.NewBMPSensor(bsbmp.BMP180)
	case "BMP280":
		return s.NewBMPSensor(bsbmp.BMP280)
	case "BMP388":
		return s.NewBMPSensor(bsbmp.BMP388)
	case "SHT30", "SHT31", "SHT35":
		return s.NewSHT3xSensor()
	default:
		return nil, fmt.Errorf("Invalid/Unsupported sensor model '%s'!", s.Model)
	}
}

func (s SensorFlags) String() string {
	var b strings.Builder
	b.WriteString(s.Model)
	if s.Address != nil {
		fmt.Fprintf(&b, ",address=0x%x", *s.Address)
	}
	if s.Bus != nil {
		fmt.Fprintf(&b, ",bus=%d", *s.Bus)
	}
	if s.Repeatability != "" {
		fmt.Fprintf(&b, ",repeatability=%s", s.Repeatability)
	}
	if s.TempOffset != 0.0 {
		fmt.Fprintf(&b, ",temp_offset=%g", s.TempOffset)
	}
	if s.HumidityOffset != 0.0 {
		fmt.Fprintf(&b, ",humidity_offset=%g", s.HumidityOffset)
	}
	return b.String()
}

type sensorCollector struct {
	Sensor          Sensor
	Up              *prometheus.Desc
	TemperatureC    *prometheus.Desc
	HumidityRH      *prometheus.Desc
	HumidityGram    *prometheus.Desc
	RawTemperatureC *prometheus.Desc
	RawHumidityRH   *prometheus.Desc
	RawHumidityGram *prometheus.Desc
	TempOffset      float64
	HumidityOffset  float64
}

func NewSensorCollector(s Sensor, tempOffset float64, humidityOffset float64) *sensorCollector {
	labels := s.Labels()
	return &sensorCollector{
		Sensor: s,
		TemperatureC: prometheus.NewDesc(
			"sensor_temperature_celsius",
			"Temperature in Celsius",
			nil,
			labels,
		),
		HumidityRH: prometheus.NewDesc(
			"sensor_humidity_percent",
			"Relative humidity in percent",
			nil,
			labels,
		),
		HumidityGram: prometheus.NewDesc(
			"sensor_humidity_grams_per_cubic_meter",
			"Absolute humidity in gram / cubic meter",
			nil,
			labels,
		),
		Up: prometheus.NewDesc(
			"sensor_up",
			"Value is 1 if reading sensor date was successful, 0 otherwise.",
			nil,
			labels,
		),
		RawTemperatureC: prometheus.NewDesc(
			"sensor_raw_temperature_celsius",
			"Uncorrected temperature in Celsius",
			nil,
			labels,
		),
		RawHumidityRH: prometheus.NewDesc(
			"sensor_raw_humidity_percent",
			"Uncorrected relative humidity in percent",
			nil,
			labels,
		),
		RawHumidityGram: prometheus.NewDesc(
			"sensor_raw_humidity_grams_per_cubic_meter",
			"Uncorrected absolute humidity in gram / cubic meter",
			nil,
			labels,
		),
		TempOffset:     tempOffset,
		HumidityOffset: humidityOffset,
	}
}

func (collector *sensorCollector) Collect(ch chan<- prometheus.Metric) {
	readings, err := collector.Sensor.Poll()
	if err != nil {
		logrus.Print(err)
		ch <- prometheus.MustNewConstMetric(collector.Up, prometheus.GaugeValue, 0.0)
	} else {
		ch <- prometheus.MustNewConstMetric(collector.Up, prometheus.GaugeValue, 1)
	}
	if readings.temperature != nil {
		ch <- prometheus.MustNewConstMetric(
			collector.TemperatureC,
			prometheus.GaugeValue,
			*readings.temperature+collector.TempOffset,
		)
		ch <- prometheus.MustNewConstMetric(
			collector.RawTemperatureC,
			prometheus.GaugeValue,
			*readings.temperature,
		)
	}
	if readings.humidity != nil {
		ch <- prometheus.MustNewConstMetric(
			collector.HumidityRH,
			prometheus.GaugeValue,
			*readings.humidity+collector.HumidityOffset,
		)
		ch <- prometheus.MustNewConstMetric(
			collector.RawHumidityRH,
			prometheus.GaugeValue,
			*readings.humidity,
		)
		if readings.temperature != nil {
			absoluteHumidity := Relative2AbsoluteHumidity(
				*readings.humidity+collector.HumidityOffset,
				*readings.temperature+collector.TempOffset,
			)
			ch <- prometheus.MustNewConstMetric(
				collector.HumidityGram,
				prometheus.GaugeValue,
				round64(absoluteHumidity, 2),
			)
			rawAbsoluteHumidity := Relative2AbsoluteHumidity(
				*readings.humidity,
				*readings.temperature,
			)
			ch <- prometheus.MustNewConstMetric(
				collector.RawHumidityGram,
				prometheus.GaugeValue,
				round64(rawAbsoluteHumidity, 2),
			)
		}
	}
}

func (collector *sensorCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.TemperatureC
	ch <- collector.HumidityRH
	ch <- collector.HumidityGram
	ch <- collector.Up
	ch <- collector.RawTemperatureC
	ch <- collector.RawHumidityRH
	ch <- collector.RawHumidityGram
}

func parseSensors(args []string) ([]SensorFlags, error) {
	sensors := make([]SensorFlags, len(args))

	for i, arg := range args {
		sensor, err := parseSensorFlags(arg)
		if err != nil {
			return nil, fmt.Errorf("sensor %d '%s': %w", i+1, arg, err)
		}
		sensors[i] = sensor
	}

	return sensors, nil
}

func round64(value float64, precision int) float64 {
	return math.Round(value*math.Pow10(precision)) / math.Pow10(precision)
}

func main() {
	listenAddress := pflag.String(
		"web.listen-address", ":9775", "Address on which to expose metrics and web interface.",
	)
	metricsPath := pflag.String(
		"web.telemetry-path", "/metrics", "Path under which to expose metrics.",
	)
	pflag.Parse()
	sensors, err := parseSensors(pflag.Args())
	if err != nil {
		logrus.Fatal(err)
	}

	logger.ChangePackageLogLevel("bsbmp", logger.InfoLevel)
	logger.ChangePackageLogLevel("i2c", logger.InfoLevel)
	logger.ChangePackageLogLevel("sht3x", logger.InfoLevel)

	for _, flags := range sensors {
		sensor, err := flags.NewSensor()
		if err != nil {
			logrus.Fatal(err)
		}
		collector := NewSensorCollector(sensor, flags.TempOffset, flags.HumidityOffset)
		prometheus.MustRegister(collector)
	}
	prometheus.MustRegister(versioncollector.NewCollector("sensor_exporter"))

	logrus.Infof(
		"Serving Prometheus sensor exporter on %s%s - for example http://localhost%s%s",
		*listenAddress,
		*metricsPath,
		*listenAddress,
		*metricsPath,
	)
	http.Handle(*metricsPath, promhttp.Handler())
	logrus.Fatal(http.ListenAndServe(*listenAddress, nil))
}
