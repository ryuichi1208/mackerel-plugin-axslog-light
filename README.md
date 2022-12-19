# mackerel-plugin-axslog-light

## Overview

This tool is a Mackerel plug-in that calculates the processing time of nginx from the access log, based on "kazeburo/mackerel-plugin-axslog". Only ltsv format is supported for log format (json, there are no plans to support other formats).By specifying the "request_time" and "upstream_response_time" keys, the actual processing time of nginx is measured.

## Usage

```
Usage:
  mackerel-plugin-axslog-light [OPTIONS]

Application Options:
      --logfile=           path to nginx ltsv logfiles. multiple log files can be specified, separated by commas.
      --key-prefix=        Metric key prefix
      --request-time-key=  key name for request_time (default: request_time)
      --upstream-time-key= key name for upstream_response_time (default: upstream_response_time)
      --filter=            text for filtering log

Help Options:
  -h, --help               Show this help message
```

## Install

``` shell
$ mkr plugin install ryuichi1208/mackerel-plugin-axslog-light
```
