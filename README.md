prometheus-sensor-exporter
==========================

`prometheus-sensor-exporter` is a [Prometheus](https://prometheus.io/) exporter
for temperature and humidity sensors.

Supported sensors
-----------------

* BME280
* SHT35

Supported flags
---------------

* address (uint8)
* bus (int)
* repeatablility: low, medium, high
* temp_offset (float64)
* humidity_offset (float64)

Build
=====

```
go get
go build
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
