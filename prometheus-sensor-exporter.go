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

type SensorFlags struct {
	Model          string
	Address        *uint8
	Bus            *int
	Repeatability  string
	TempOffset     float64
	HumidityOffset float64
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
		fmt.Fprintf(&b, ",repeatablility=%s", s.Repeatability)
	}
	if s.TempOffset != 0.0 {
		fmt.Fprintf(&b, ",temp_offset=%g", s.TempOffset)
	}
	if s.HumidityOffset != 0.0 {
		fmt.Fprintf(&b, ",humidity_offset=%g", s.HumidityOffset)
	}
	return b.String()
}

type Readings struct {
	temperature *float64
	humidity    *float64
}

type Sensor interface {
	Poll() (Readings, error)
	Labels() prometheus.Labels
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
	Address uint8
	Bus     int
	Model   string
	bme     *bsbmp.BMP
	mutex   sync.Mutex
}

func NewSensor(address uint8, bus int, model string, repeatability sht3x.MeasureRepeatability, repeatability_str string) *SHT3xSensor {
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

func NewBME280Sensor(address uint8, bus int, model string) *BME280Sensor {
	fmt.Printf("New sensor: %s,address=0x%x,bus=%d\n", model, address, bus)

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
		Address: address,
		Bus:     bus,
		Model:   model,
		bme:     bme,
	}
}

func SensorFromFlags(flags SensorFlags) *SHT3xSensor {
	// Defaults
	if flags.Address == nil {
		*flags.Address = 0x45
	}
	if flags.Bus == nil {
		*flags.Bus = 0
	}
	if flags.Repeatability == "" {
		flags.Repeatability = "high"
	}

	var repeatability sht3x.MeasureRepeatability
	switch flags.Repeatability {
	case "low":
		repeatability = sht3x.RepeatabilityLow
	case "medium":
		repeatability = sht3x.RepeatabilityMedium
	case "high":
		repeatability = sht3x.RepeatabilityHigh
	default:
		log.Fatalf("Unknown repeatability: %s", flags.Repeatability)
	}

	return NewSensor(*flags.Address, *flags.Bus, flags.Model, repeatability, flags.Repeatability)
}

func BME280SensorFromFlags(flags SensorFlags) *BME280Sensor {
	// Defaults
	if flags.Address == nil {
		*flags.Address = 0x76
	}
	if flags.Bus == nil {
		*flags.Bus = 0
	}

	return NewBME280Sensor(*flags.Address, *flags.Bus, flags.Model)
}

type sensorCollector struct {
	Sensor         Sensor
	Up             *prometheus.Desc
	TemperatureC   *prometheus.Desc
	HumidityRH     *prometheus.Desc
	HumidityGram   *prometheus.Desc
	RawTempC       *prometheus.Desc
	RawHumidityRH  *prometheus.Desc
	TempOffset     float64
	HumidityOffset float64
}

func NewSensorCollector(s Sensor, tempOffset float64, humidityOffset float64) *sensorCollector {
	labels := s.Labels()
	return &sensorCollector{
		Sensor: s,
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
		RawTempC: prometheus.NewDesc("sensor_raw_temperature_celsius",
			"The uncorrected temperature in Celsius",
			nil,
			labels,
		),
		RawHumidityRH: prometheus.NewDesc("sensor_raw_humidity_percent",
			"Uncorrected relative humidity in percent",
			nil,
			labels,
		),
		TempOffset:     tempOffset,
		HumidityOffset: humidityOffset,
	}
}

func (collector *sensorCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.TemperatureC
	ch <- collector.HumidityRH
	ch <- collector.HumidityGram
	ch <- collector.Up
}

func round64(value float64, precision int) float64 {
	value2 := math.Round(value*math.Pow10(precision)) / math.Pow10(precision)
	return value2
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
	readings, err := collector.Sensor.Poll()
	if err != nil {
		log.Print(err)
		ch <- prometheus.MustNewConstMetric(collector.Up, prometheus.GaugeValue, 0.0)
	} else {
		ch <- prometheus.MustNewConstMetric(collector.Up, prometheus.GaugeValue, 1)
	}
	if readings.temperature != nil {
		ch <- prometheus.MustNewConstMetric(collector.TemperatureC, prometheus.GaugeValue, *readings.temperature+collector.TempOffset)
		ch <- prometheus.MustNewConstMetric(collector.RawTempC, prometheus.GaugeValue, *readings.temperature)
	}
	if readings.humidity != nil {
		ch <- prometheus.MustNewConstMetric(collector.HumidityRH, prometheus.GaugeValue, *readings.humidity+collector.HumidityOffset)
		ch <- prometheus.MustNewConstMetric(collector.RawHumidityRH, prometheus.GaugeValue, *readings.humidity)
		if readings.temperature != nil {
			absoluteHumidity := Relative2AbsoluteHumidity(*readings.humidity+collector.HumidityOffset, *readings.temperature+collector.TempOffset)
			ch <- prometheus.MustNewConstMetric(
				collector.HumidityGram,
				prometheus.GaugeValue,
				round64(absoluteHumidity, 2),
			)
		}
	}
}

func (s BME280Sensor) Labels() prometheus.Labels {
	// FIXME: drop "repeatability"
	return prometheus.Labels{
		"address":       fmt.Sprintf("0x%x", s.Address),
		"bus":           fmt.Sprintf("%d", s.Bus),
		"model":         s.Model,
		"repeatability": "",
	}
}

func (s BME280Sensor) Poll() (Readings, error) {
	var readings Readings

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
	temp2 := round64(float64(temp), 2)
	readings.temperature = &temp2

	s.mutex.Lock()
	_, rh, err := s.bme.ReadHumidityRH(bsbmp.ACCURACY_STANDARD)
	s.mutex.Unlock()
	//}
	if err != nil {
		return readings, err
	}

	rh2 := round64(float64(rh), 2)
	readings.humidity = &rh2
	return readings, nil

	// TODO:
	// sensor.ReadPressurePa
	// sensor.ReadPressureMmHg
	// sensor.ReadAltitude
}

func parseSensorFlags(sensor string) (SensorFlags, error) {
	var flags SensorFlags
	fields := strings.Split(sensor, ",")
	//model := fields[0]
	flags.Model = fields[0]
	//fmt.Printf("Model: %v\n", model)
	//m := make(map[string]string, len(fields[1:]))
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
				return flags, fmt.Errorf("Specified address '%s' is not an unsigned integer: %s", value, err)
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

	sensors := make([]SensorFlags, flag.NArg())
	for i, sensor := range flag.Args() {
		fmt.Printf("Sensor: %q\n", sensor)
		//fmt.Printf("flags: %v\n", m)
		//fmt.Printf("\n")
		var err error
		sensors[i], err = parseSensorFlags(sensor)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("sensor[%d] = %s\n", i, sensors[i])
	}

	for _, flags := range sensors {
		var sensor Sensor
		switch flags.Model {
		case "SHT35":
			sensor = SensorFromFlags(flags)
		case "BME280":
			sensor = BME280SensorFromFlags(flags)
		default:
			log.Fatalf("Invalid model '%s'!", flags.Model)
			continue
		}

		collector := NewSensorCollector(sensor, flags.TempOffset, flags.HumidityOffset)
		prometheus.MustRegister(collector)
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
