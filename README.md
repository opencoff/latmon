# latmon

`latmon` is a http/https latency monitor that saves dns, tcp, tls
and http latencies to one or more sites.

Features:

* outputs latencies as a csv file
* generates interactive charts (`go-echarts`)
* http and https support
* customizable ping interval
* always generates an 24-hour report (csv + charts)
* by default saves intermediate results every 3600 samples

## How to build it
Pre-requisites:

    * go toolchain 1.22+
    * bash 4.x+
    * (optional) make or GNU Make

If you have GNU Make: `make` will build the binary

If you don't have GNU Make, `./build -s` will build the binary

In either case, the binary will be in the directory `./bin/$machine/`
where `$machine` is of the form `$OS-$ARCH`. eg if you are building
this on an linux-amd64 host - the binary will be in
`./bin/linux-amd64`.

## Usage
    latmon [options] HOST [HOST..]

    Where HOST is of the form:

        https:hostname:port

    hostname - can be either an IP address or hostname.

    Options:
      -b, --batch-size int   Collect 'B' samples per measurement run (default 3600)
      -i, --every I          Send pings every I interval apart (default 2s)
      -h, --help             Show this help message and exit
      -L, --log L            Send logs to destination L (default "SYSLOG")
          --log-level P      Log at priority P (default "INFO")
      -d, --output-dir D     Put charts in directory D (default ".")
      -t, --timeout T        Set rx deadline to T seconds (default 2s)
          --version          Show program version and exit

Example invocation:

    latmon -i 3s -d /tmp/latmon -L /tmp/latmon/latmon.log \
            --log-level DEBUG https:www.google.com

Latmon puts charts for each host in a subdir named after
the host. The csv files are stored in the `csv` subdir of each host dir
and the charts are stored in the `html` subdir of each host dir.
Daily stats and charts are stored in files with the format
*YYYY-MM-DD.csv* and *YY-MM-DD.html* respectively.

# TODO
1. Add support for quic/http
2. Add support for icmp (maybe)

# Guide to Source
* latmon uses a simple http client in `internal/http`
* the plotting aspect is in `internal/plot`
* `src/http.go` periodically pings a host and sends latency
  measurements via chan. Each monitored host will have an instance
  of `hping`.
