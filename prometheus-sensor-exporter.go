package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	i2c "github.com/d2r2/go-i2c"
	sht3x "github.com/d2r2/go-sht3x"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

type sensorsCollector struct {
	Sensor       *Sensor
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
	}
}

func (collector *sensorsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.TemperatureC
	ch <- collector.HumidityRH
}

func (collector *sensorsCollector) Collect(ch chan<- prometheus.Metric) {
	var temp, rh float32
	if collector.Sensor.Bus == 0 {
		temp = 20.0
		rh = 50.0
		time.Sleep(100 * time.Millisecond)
	} else {
		var err error
		temp, rh, err = collector.Sensor.SHT3X.ReadTemperatureAndRelativeHumidity(collector.Sensor.I2C, sht3x.RepeatabilityLow)
		if err != nil {
			log.Fatal(err)
		}
	}

	ch <- prometheus.MustNewConstMetric(collector.TemperatureC, prometheus.GaugeValue, float64(temp))
	ch <- prometheus.MustNewConstMetric(collector.HumidityRH, prometheus.GaugeValue, float64(rh))
}

func main() {
	listen_address := flag.String("web.listen-address", ":8004", "Address on which to expose metrics and web interface.")
	telemetry_path := flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	flag.Parse()
	fmt.Printf("web.listen-address = %q, web.telemetry-path = %q\n", *listen_address, *telemetry_path)

	for _, sensor := range flag.Args() {
		fmt.Printf("Sensor: %q\n", sensor)
		fields := strings.Split(sensor, ",")
		model := fields[0]
		fmt.Printf("Model: %v\n", model)
		m := make(map[string]string)
		for _, field := range fields[1:] {
			key_value := strings.SplitN(field, "=", 2)
			m[key_value[0]] = key_value[1]
		}
		fmt.Printf("flags: %v\n", m)
		fmt.Printf("\n")
	}

	//sensorPtr := flag.String("sensor", "foo", "Sensor to scrape. <model>[,bus=<n>,address=<0xn>]")
	// TODO: Command line parsing (supporting multiple sensors)

	// bus=1
	// address=0x45
	// port=8004
	// Model: SHT35
	var sensor0 = NewSensor(0x42, 0, "fake-model")
	var sensor1 = NewSensor(0x45, 1, "SHT35")

	var collector0 = NewSensorsCollector(sensor0)
	prometheus.MustRegister(collector0)
	var collector1 = NewSensorsCollector(sensor1)
	prometheus.MustRegister(collector1)

	http.Handle(*telemetry_path, promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listen_address, nil))
}
