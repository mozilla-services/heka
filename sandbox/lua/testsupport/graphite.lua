require "string"
require "math"
require "table"
require "cjson"

local graphite = require "graphite"

local status_codes = {}
local request_times = {}

local ticker_interval = 10
local percent_thresh = 90

function process_message()
    local hostname = read_message("Hostname")
    local logger = read_message("Logger")
    local status = read_message("Fields[status]")
    local request_time = read_message("Fields[request_time]")

    local bucket = string.format("%s.nginx.%s.http_%d", hostname, logger, status)
    local val = status_codes[bucket] or 0
    status_codes[bucket] = val + 1

    bucket = string.format("%s.nginx.%s.request_time", hostname, logger)
    val = request_times[bucket] or {}
    val[#val+1] = request_time
    request_times[bucket] = val
    return 0
end

function timer_event(ns)
    local now_sec = graphite.ns_to_sec(ns)
    local num_stats = 0

    for bucket, count in pairs(status_codes) do
        num_stats = num_stats + 1
    end

    add_to_payload(graphite.multi_counts_rates(status_codes, ticker_interval, now_sec))
    add_to_payload(graphite.multi_timeseries_metrics(request_times, ticker_interval, percent_thresh, now_sec))

    for bucket, times in pairs(request_times) do
        num_stats = num_stats + 1
    end

    add_to_payload(string.format("stats.statsd.numStats %d %d\n", num_stats, now_sec))
    inject_payload("txt", "statmetric")
end
