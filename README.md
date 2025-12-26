prometheus-sensor-exporter
==========================

`prometheus-sensor-exporter` is a [Prometheus](https://prometheus.io/) exporter
for temperature and humidity sensors.

Supported sensors
-----------------

* Bosch Sensortec BME280, BMP180, BMP280, BMP388 (using https://github.com/d2r2/go-bsbmp)
* Sensirion SHT30, SHT31, SHT35 (using https://github.com/d2r2/go-sht3x)

Supported flags
---------------

* address: I²C address (uint8)
* bus: I²C bus number (int)
* repeatability: low, medium, or high (only SHT sensors)
* temp_offset: fixed temperature offset in °C to add (float64)
* humidity_offset: fixed humidity offset in percent to add (float64)

Example usage
-------------

```
prometheus-sensor-exporter BME280,bus=0 SHT35,bus=1,address=0x45
```

Build
=====

```
go get
go build -ldflags "\
    -X github.com/prometheus/common/version.Branch=$(git branch --show-current) \
    -X github.com/prometheus/common/version.Revision=$(git rev-parse --short HEAD) \
    -X github.com/prometheus/common/version.Version=0.1.0"
```

Test
====

```
go test -coverprofile=coverage.out -v
go tool cover -html=coverage.out -o coverage.html
```

Formatting
==========

```
goimports -w .
go fmt
```
