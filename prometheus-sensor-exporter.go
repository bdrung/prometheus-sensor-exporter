package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	i2c "github.com/d2r2/go-i2c"
	sht3x "github.com/d2r2/go-sht3x"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

type Sensor struct {
	Address uint8
	Bus     int
	Model   string
	I2C     *i2c.I2C
	SHT3X   sht3x.SHT3X
}

func NewSensor(address uint8, bus int, model string) *Sensor {
	fmt.Printf("New sensor: %s,address=0x%x,bus=%d\n", model, address, bus)
	i2c, err := i2c.NewI2C(address, bus)
	if err != nil {
		log.Fatal(err)
	}
	return &Sensor{
		Address: address,
		Bus:     bus,
		Model:   model,
		I2C:     i2c,
		SHT3X:   *sht3x.NewSHT3X(),
	}
}

func SensorFromMap(model string, fields map[string]string) *Sensor {
	// Defaults
	var address8 uint8 = 0x45
	var bus int = 0

	if address, ok := fields["address"]; ok {
		address64, _ := strconv.ParseUint(address, 0, 8)
		address8 = uint8(address64)
	} else {
		log.Println("unknown address:", address)
	}

	if bus_str, ok := fields["bus"]; ok {
		bus32, _ := strconv.ParseInt(bus_str, 0, 32)
		bus = int(bus32)
	} else {
		log.Println("unknown bus:", bus_str)
	}

	return NewSensor(address8, bus, model)
}

type sensorsCollector struct {
	Sensor       *Sensor
	Up           *prometheus.Desc
	TemperatureC *prometheus.Desc
	HumidityRH   *prometheus.Desc
}

func NewSensorsCollector(c *Sensor) *sensorsCollector {
	labels := prometheus.Labels{"address": fmt.Sprintf("0x%x", c.Address), "bus": fmt.Sprintf("%d", c.Bus), "model": c.Model}
	return &sensorsCollector{
		Sensor: c,
		TemperatureC: prometheus.NewDesc("sensor_temperature_celsius",
			"The temperature in Celsius",
			nil,
			labels,
		),
		HumidityRH: prometheus.NewDesc("sensor_humidity_percent",
			"Relative humidity in percent",
			nil,
			labels,
		),
		Up: prometheus.NewDesc("sensors_up",
			"TODO",
			nil,
			labels,
		),
	}
}

func (collector *sensorsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.TemperatureC
	ch <- collector.HumidityRH
	ch <- collector.Up
}

func (collector *sensorsCollector) Collect(ch chan<- prometheus.Metric) {
	temp, rh, err := collector.Sensor.SHT3X.ReadTemperatureAndRelativeHumidity(collector.Sensor.I2C, sht3x.RepeatabilityLow)
	if err != nil {
		log.Print(err)
		ch <- prometheus.MustNewConstMetric(collector.Up, prometheus.GaugeValue, 0.0)
		return
	}

	ch <- prometheus.MustNewConstMetric(collector.TemperatureC, prometheus.GaugeValue, float64(temp))
	ch <- prometheus.MustNewConstMetric(collector.HumidityRH, prometheus.GaugeValue, float64(rh))
	ch <- prometheus.MustNewConstMetric(collector.Up, prometheus.GaugeValue, 1.0)
}

func main() {
	listenAddress := flag.String(
		"web.listen-address", ":9775", "Address on which to expose metrics and web interface.",
	)
	metricsPath := flag.String(
		"web.telemetry-path", "/metrics", "Path under which to expose metrics.",
	)
	flag.Parse()

	// TODO: ab hier
	fmt.Printf("web.listen-address = %q, web.telemetry-path = %q\n", *listenAddress, *metricsPath)

	for _, sensor := range flag.Args() {
		fmt.Printf("Sensor: %q\n", sensor)
		fields := strings.Split(sensor, ",")
		model := fields[0]
		fmt.Printf("Model: %v\n", model)
		m := make(map[string]string, len(fields[1:]))
		for _, field := range fields[1:] {
			key_value := strings.SplitN(field, "=", 2)
			if len(key_value) == 2 {
				m[key_value[0]] = key_value[1]
			} else {
				m[key_value[0]] = ""
			}
		}
		fmt.Printf("flags: %v\n", m)
		fmt.Printf("\n")

		switch model {
		case "SHT35":
			sensor := SensorFromMap(model, m)
			collector := NewSensorsCollector(sensor)
			prometheus.MustRegister(collector)
		default:
			log.Fatal("Invalid model '%s'!", model)
		}
	}

	//sensorPtr := flag.String("sensor", "foo", "Sensor to scrape. <model>[,bus=<n>,address=<0xn>]")
	// TODO: Command line parsing (supporting multiple sensors)

	// bus=1
	// address=0x45
	// port=8004
	// Model: SHT35
	//var sensor0 = NewSensor(0x42, 0, "fake-model")
	//var sensor1 = NewSensor(0x45, 1, "SHT35")

	//var collector0 = NewSensorsCollector(sensor0)
	//prometheus.MustRegister(collector0)
	//var collector1 = NewSensorsCollector(sensor1)
	//prometheus.MustRegister(collector1)

	// TODO: bis hier
	http.Handle(*metricsPath, promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}