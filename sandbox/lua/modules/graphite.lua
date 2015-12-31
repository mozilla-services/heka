-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
Module contains various utility functions for metrics stored Graphite.

Currently module focuses on generators of metrics which can be later passed
to Graphite.

API
^^^

**count_rate(bucket, count, ticker_interval, now_sec)**
    Generates string with count and rate metric for graphite.

    *Arguments*
        - bucket - node name in which metric will be stored.
        - count - value of count.
        - ticker_interval - base interval for calculation of rate.
        - now_sec - timestamp (float) for metric.

    *Return*
        String with count and rate metric for `stats.counters.<bucket>.count` and
        `stats.counters.<bucket>.rate` bucket with time given in <now_sec>.

**multi_counts_rates(counts, ticker_interval, now_sec)**
    Generates a multiline graphite count metric with their rates.

    *Arguments*
        - counts - table, indices will be mapped to buckets and values to their
                   specific counts e.g {'bucket1': 1, 'bucket2':2}
        - ticker_interval - base interval for calculation of rate.
        - now_sec - timestamp (float) for metric.

    *Return*
        String with multiple counts and rates returned via `return_count` function.

**timeseries_metrics(bucket, times, ticker_interval, percent_thresh, now_sec)**
    Generates string with metrics for given timeseries data to pass it to graphite.

    *Arguments*
        - bucket - node name in which metric will be stored.
        - times - a table with times (float values).
        - ticker_interval - base interval for calculation of rate.
        - percent_treshold - base treshould for percentiles.
        - now_sec - timestamp (float) for metric.

    *Return*
        Returns multiline graphite string with following metrics:
             - stats.timers.<bucket>.count
             - stats.timers.<bucket>.rate
             - stats.timers.<bucket>.min
             - stats.timers.<bucket>.max
             - stats.timers.<bucket>.mean
             - stats.timers.<bucket>.mean_percentile
             - stats.timers.<bucket>.upper_percentile

**multi_timeseries_metrics(timers, ticker_interval, percent_thresh, now_sec)**
    Returns multline string with stats calculated for timeseries in their respective buckets

    *Arguments*
        - timers - tables with bucket names and tables of times inside e.g {'bucket': [1,2,3,4]}
        - ticker_interval - base interval for calculation of rate.
        - percent_treshold - base treshould for percentiles.
        - now_sec - timestamp (float) for metric.

    *Return*
        String with corresponding metric series and their respective buckets defined in timers table.
        Metrics are the same as for the `timeseries_metrics_function`.

**function ns_to_sec(ns)**
    Converts nanoseconds into seconds.

    *Arguments*
        - ns - nanoseconds

    *Return*
        Seconds in float value.
--]]

-- Imports
local string = require "string"
local math = require "math"
local table = require "table"

--[[ Removes external access for the rest of modules. --]]
local M = {}
setfenv(1, M)

--[[ Public interface --]]
function count_rate(bucket, count, ticker_interval, now_sec)
	return string.format("stats.counters.%s.count %d %d\nstats.counters.%s.rate %f %d\n",
						 bucket, count, now_sec,
						 bucket, count/ticker_interval, now_sec)
end

function multi_counts_rates(counts, ticker_interval, now_sec)
	local stats = {}
	for bucket, count in pairs(counts) do
		stats[#stats+1] = count_rate(bucket, count, ticker_interval, now_sec)
	end
	return table.concat(stats)
end

function timeseries_metrics(bucket, times, ticker_interval, percent_thresh, now_sec)
	local stats
 	local count, min, max, sum, mean, rate, mean_percentile, upper_percentile

	local cumulative, tmp 
    count = #times
    if count == 0 then
        min = 0
        max = 0
        sum = 0
        mean = 0
        rate = 0
        mean_percentile = 0
        upper_percentile = 0
    else
        rate = count / ticker_interval
        table.sort(times)
        min = times[1] 
        max = times[count]
        mean = min
        local thresh_bound = max

        cumulative = {}
        cumulative[0] = 0
        for i, time in ipairs(times) do
            cumulative[i] = cumulative[i-1] + time
        end

        if count > 1 then
            tmp = ((100 - percent_thresh) / 100) * count
            local num_in_thresh = count - math.floor(tmp+.5)
            if num_in_thresh > 0 then
                mean = cumulative[num_in_thresh] / num_in_thresh
                thresh_bound = times[num_in_thresh]
            else
                mean = min
                thresh_bound = max
            end
        end
        mean_percentile = mean
        upper_percentile = thresh_bound
        sum = cumulative[count]
        mean = sum / count
    end

	return string.format([[stats.timers.%s.count %d %d
stats.timers.%s.count_ps %f %d
stats.timers.%s.lower %f %d
stats.timers.%s.upper %f %d
stats.timers.%s.sum %f %d
stats.timers.%s.mean %f %d
stats.timers.%s.mean_%d %f %d
stats.timers.%s.upper_%d %f %d
]],
						bucket, count, now_sec,
						bucket, rate, now_sec,
						bucket, min, now_sec,
						bucket, max, now_sec,
						bucket, sum, now_sec,
						bucket, mean, now_sec,
						bucket, percent_thresh, mean_percentile, now_sec,
						bucket, percent_thresh, upper_percentile, now_sec)
end

function multi_timeseries_metrics(timers, ticker_interval, percent_thresh, now_sec)
	local stats = {}
    for bucket, times in pairs(timers) do
        stats[#stats+1] = timeseries_metrics(bucket, times, ticker_interval, percent_thresh, now_sec)
    end
    return table.concat(stats)
end

function ns_to_sec(ns)
	return math.floor(ns / 1e9)
end

return M
