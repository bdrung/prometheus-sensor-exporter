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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

type readings struct {
	temperature *float64
	humidity    *float64
}

type sensor interface {
	poll() (readings, error)
	labels() prometheus.Labels
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

type BME280Sensor struct {
	Address        uint8
	Bus            int
	Model          string
	bme            *bsbmp.BMP
	mutex          sync.Mutex
	TempOffset     float64
	HumidityOffset float64
}

func NewSensor(address uint8, bus int, model string, repeatability sht3x.MeasureRepeatability, repeatability_str string) *SHT3xSensor {
	// TODO: temp + humidity offset
	fmt.Printf("New sensor: %s,address=0x%x,bus=%d,repeatability=%s\n", model, address, bus, repeatability_str)
	i2c, err := i2c.NewI2C(address, bus)
	if err != nil {
		log.Fatal(err)
	}
	return &SHT3xSensor{
		Address:           address,
		Bus:               bus,
		Model:             model,
		I2C:               i2c,
		SHT3X:             *sht3x.NewSHT3X(),
		repeatability:     repeatability,
		repeatability_str: repeatability_str,
	}
}

func NewBME280Sensor(address uint8, bus int, model string, tempOffset float64, humidityOffset float64) *BME280Sensor {
	fmt.Printf("New sensor: %s,address=0x%x,bus=%d,temp_offset=%f,humidity_offset=%f\n", model, address, bus, tempOffset, humidityOffset)

	// todo loglevel flag
	logger.ChangePackageLogLevel("i2c", logger.InfoLevel)
	logger.ChangePackageLogLevel("bsbmp", logger.InfoLevel)

	i2c, err := i2c.NewI2C(address, bus)
	if err != nil {
		log.Fatal(err)
	}
	bme, err := bsbmp.NewBMP(bsbmp.BME280, i2c)
	if err != nil {
		log.Fatal(err)
	}
	return &BME280Sensor{
		Address:        address,
		Bus:            bus,
		Model:          model,
		bme:            bme,
		TempOffset:     tempOffset,
		HumidityOffset: humidityOffset,
	}
}

func SensorFromMap(model string, fields map[string]string) *SHT3xSensor {
	// Defaults
	var address8 uint8 = 0x45
	var bus int = 0
	var repeatability sht3x.MeasureRepeatability = sht3x.RepeatabilityHigh
	var repeatability_str2 string = "high"

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

	if repeatability_str, ok := fields["repeatability"]; ok {
		switch repeatability_str {
		case "low":
			repeatability = sht3x.RepeatabilityLow
		case "medium":
			repeatability = sht3x.RepeatabilityMedium
		case "high":
			repeatability = sht3x.RepeatabilityHigh
		default:
			log.Fatalf("Unknown repeatability: %s", repeatability_str)
		}
		repeatability_str2 = repeatability_str
	}

	return NewSensor(address8, bus, model, repeatability, repeatability_str2)
}

func BME280SensorFromMap(model string, fields map[string]string) *BME280Sensor {
	// Defaults
	var address8 uint8 = 0x76
	var bus int = 0
	var temp_offset float64 = 0
	var humidity_offset float64 = 0

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

	if temp_offset_str, ok := fields["temp_offset"]; ok {
		var err error
		temp_offset, err = strconv.ParseFloat(temp_offset_str, 64)
		if err != nil {
			log.Println("Failed to parse temperature offset '%s': %s", temp_offset_str, err)
		}
	}

	if humidity_offset_str, ok := fields["humidity_offset"]; ok {
		var err error
		humidity_offset, err = strconv.ParseFloat(humidity_offset_str, 64)
		if err != nil {
			log.Println("Failed to parse humidity offset '%s': %s", humidity_offset_str, err)
		}
	}

	return NewBME280Sensor(address8, bus, model, temp_offset, humidity_offset)
}

type sensorCollector struct {
	Sensor       sensor
	Up           *prometheus.Desc
	TemperatureC *prometheus.Desc
	HumidityRH   *prometheus.Desc
	HumidityGram *prometheus.Desc
}

func NewSensorCollector(c sensor) *sensorCollector {
	labels := c.labels()
	return &sensorCollector{
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
		HumidityGram: prometheus.NewDesc("sensor_humidity_grams_per_cubic_meter",
			"Absolute humidity in gram / cubic meter",
			nil,
			labels,
		),
		Up: prometheus.NewDesc("sensor_up",
			"Value is 1 if reading sensor date was successful, 0 otherwise.",
			nil,
			labels,
		),
	}
}

func (collector *sensorCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.TemperatureC
	ch <- collector.HumidityRH
	ch <- collector.HumidityGram
	ch <- collector.Up
}

func round64(value float64, precision int) float64 {
	value2 := math.Round(value*math.Pow10(precision)) /
		math.Pow10(precision)
	return value2
}

func (s SHT3xSensor) labels() prometheus.Labels {
	return prometheus.Labels{
		"address":       fmt.Sprintf("0x%x", s.Address),
		"bus":           fmt.Sprintf("%d", s.Bus),
		"model":         s.Model,
		"repeatability": s.repeatability_str,
	}
}

func (s SHT3xSensor) poll() (readings, error) {
	var readings readings

	//if collector.Sensor.Bus == 0 {
	//	temp = 20.0
	//	rh = 50.0
	//	time.Sleep(100 * time.Millisecond)
	//	err = errors.New("prometheus-sensor-exporter: Fake failure")
	//} else {
	s.mutex.Lock()
	temp, rh, err := s.SHT3X.ReadTemperatureAndRelativeHumidity(s.I2C, s.repeatability)
	s.mutex.Unlock()
	if err != nil {
		return readings, err
	}

	temp2, rh2 := round64(float64(temp), 2), round64(float64(rh), 2)

	readings.temperature = &temp2
	readings.humidity = &rh2
	return readings, nil
}

func (collector *sensorCollector) Collect(ch chan<- prometheus.Metric) {
	readings, err := collector.Sensor.poll()
	if err != nil {
		log.Print(err)
		ch <- prometheus.MustNewConstMetric(collector.Up, prometheus.GaugeValue, 0.0)
	} else {
		ch <- prometheus.MustNewConstMetric(collector.Up, prometheus.GaugeValue, 1)
	}
	if readings.temperature != nil {
		ch <- prometheus.MustNewConstMetric(collector.TemperatureC, prometheus.GaugeValue, *readings.temperature)
	}
	if readings.humidity != nil {
		ch <- prometheus.MustNewConstMetric(collector.HumidityRH, prometheus.GaugeValue, *readings.humidity)
		if readings.temperature != nil {
			ch <- prometheus.MustNewConstMetric(
				collector.HumidityGram,
				prometheus.GaugeValue,
				Relative2AbsoluteHumidity(*readings.temperature, *readings.humidity),
			)
		}
	}
}

func (s BME280Sensor) labels() prometheus.Labels {
	// FIXME: drop "repeatability"
	return prometheus.Labels{
		"address":       fmt.Sprintf("0x%x", s.Address),
		"bus":           fmt.Sprintf("%d", s.Bus),
		"model":         s.Model,
		"repeatability": "",
	}
}

func (s BME280Sensor) poll() (readings, error) {
	var readings readings

	//var temp, rh float32
	//var err error
	//if collector.Sensor.Bus == 0 {
	//	temp = 20.0
	//	rh = 50.0
	//	time.Sleep(100 * time.Millisecond)
	//	err = errors.New("prometheus-sensor-exporter: Fake failure")
	//} else {
	s.mutex.Lock()
	temp, err := s.bme.ReadTemperatureC(bsbmp.ACCURACY_STANDARD)
	s.mutex.Unlock()
	//}
	if err != nil {
		return readings, err
	}
	temp2 := round64(float64(temp)+s.TempOffset, 2)
	readings.temperature = &temp2

	s.mutex.Lock()
	_, rh, err := s.bme.ReadHumidityRH(bsbmp.ACCURACY_STANDARD)
	s.mutex.Unlock()
	//}
	if err != nil {
		return readings, err
	}

	rh2 := round64(float64(rh)+s.HumidityOffset, 2)
	readings.humidity = &rh2
	return readings, nil

	// TODO:
	// sensor.ReadPressurePa
	// sensor.ReadPressureMmHg
	// sensor.ReadAltitude
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
	// TODO: find good logging library
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
			collector := NewSensorCollector(sensor)
			prometheus.MustRegister(collector)
		case "BME280":
			sensor := BME280SensorFromMap(model, m)
			collector := NewSensorCollector(sensor)
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

	//var collector0 = NewSensorCollector(sensor0)
	//prometheus.MustRegister(collector0)
	//var collector1 = NewSensorCollector(sensor1)
	//prometheus.MustRegister(collector1)

	// TODO: bis hier
	http.Handle(*metricsPath, promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
