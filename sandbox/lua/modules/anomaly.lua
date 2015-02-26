-- This Source Code Form is subject to the terms of the Mozilla Public
-- License, v. 2.0. If a copy of the MPL was not distributed with this
-- file, You can obtain one at http://mozilla.org/MPL/2.0/.

--[[
API
^^^
**parse_config(anomaly_config)**
    Parses the anomaly_config into a Lua table. If the configuration is invalid
    an error is thrown.

    *Arguments*
        - anomaly_config (string or nil)

        The configuration can specify any number of algorithm function calls
        (space delimited if desired, but they will also work back to back with
        no delimiter).  This allows for analysis of multiple graphs, columns,
        and even specification of multiple algorithms per column.

        **Rate of change test**

        Only use this test on data with a normal
        (Gaussian http://en.wikipedia.org/wiki/Normal_distribution) distribution.
        It identifies rapid changes (spikes) in the data (increasing and
        decreasing) but ignores cyclic data that has a more gradual rise and
        fall.  It is typically used for something like HTTP 200 status code
        analysis to detect a sudden increase/decrease in web traffic.

        *roc("payload_name", col, win, hwin, sd, loss_of_data, start_of_data)*
            - payload_name (string)
                Quoted string containing the *payload_name* value used in the
                inject_payload function call.  If the payload name contains a
                double quote it should be escaped as two double quotes in a row.

            - col (uint)
                The circular buffer column to perform the analysis on.

            - win (uint)
                The number of intervals in an analysis window.

            - hwin (uint)
                The number of intervals in the historical analysis window (0
                uses the full history). Must be greater than or equal to 'win'.

            - sd (double)
                The standard deviation threshold to trigger the anomaly.

            - loss_of_data (bool)
                Alert if data stops.

            - start_of_data (bool)
                Alert if data starts.

            e.g. roc("Output1", 1, 15, 0, 2, true, false)

        **Mann-Whitney-Wilcoxon test** http://en.wikipedia.org/wiki/Mann-Whitney

        **Parametric**

        Only use this test on data with a normal
        (Gaussian http://en.wikipedia.org/wiki/Normal_distribution) distribution.
        It identifies more gradual changes in the data (increasing, decreasing,
        or any).  It is typically used with something like server memory
        analysis where the values are more stable and gradual changes are
        interesting (e.g., memory leak).

        *mww("payload_name", col, win, nwin, pvalue, trend)*
            - payload_name (string)
                Quoted string containing the *payload_name* value used in the
                inject_payload function call.  If the payload name contains a
                double quote it should be escaped as two double quotes in a row.

            - col (uint)
                The circular buffer column to perform the analysis on.

            - win (uint)
                The number of intervals in an analysis window (should be at
                least 20).

            - nwin (uint)
                The number of analysis windows to compare.

            - pvalue (double)
                The pvalue threshold to trigger the prediction.
                http://en.wikipedia.org/wiki/P_value

            - trend (string)
                (decreasing|increasing|any)

            e.g. mww("Output1", 2, 60, 10, 0.0001, decreasing)

        **Non-parametric**

        This test can be used on data with a normal
        (Gaussian http://en.wikipedia.org/wiki/Normal_distribution)
        or non-normal
        (nonparametric http://en.wikipedia.org/wiki/Nonparametric_statistics)
        distribution.  It identifies overlap/similarities between two data sets.
        It is typically used for something like detecting an increase in HTTP
        500 status code errors.

        *mww_nonparametric("payload_name", col, win, nwin, pstat)*
            - payload_name (string)
                Quoted string containing the *payload_name* value used in the
                inject_payload function call.  If the payload name contains a
                double quote it should be escaped as two double quotes in a row.

            - col (uint)
                The circular buffer column to perform the analysis on.

            - win (uint)
                The number of intervals in an analysis window.

            - nwin (uint)
                The number of analysis windows to compare.

            - pstat (double)
                Value between 0 and 1. Anything above 0.5 is an increasing trend
                anything below 0.5 is a decreasing trend.
                http://en.wikipedia.org/wiki/Mann-Whitney#.CF.81_statistic

            e.g. mww_nonparametric("Output1", 2, 15, 10, 0.55)

    *Return*
        Configuration table if parsing was successful or nil, if nil was
        passed in.


**detect(ns, name, cbuf, anomaly_config)**
    Detects anomalies in the circular buffer data returning any error messages
    for alert generation and array of annotations for the graph.

    *Arguments*
        - ns (int64) current time in nanoseconds since the UNIX epoch. It used
          to advance the circular buffer if necessary (i.e., if no data is being
          received). The anomaly detection is always performed on the newest
          data (ignoring the current interval since it is incomplete).
        - name (string) circular buffer payload name
        - cbuf (userdata) circular buffer
        - anomaly_config (table) returned from the parse() method

    *Return*
        - string if an anomaly was detected, otherwise nil.
        - array of annotation tables
--]]

-- Imports
local annotation= require "annotation"
local l         = require "lpeg"
l.locale(l)
local math      = require "math"
local string    = require "string"
local table     = require "table"
local error     = error
local ipairs    = ipairs
local tonumber  = tonumber

local M = {}
setfenv(1, M) -- Remove external access to contain everything in the module

--[[
Detect a changing trend in a normal data distribution using the Mann-Whitney U test.

Arguments:

- name (string)
    The name of the circular buffer

- cbuf (circular buffer userdata object)

- cfg (table)
    - algorithm (string)
        "mww"

    - configuration args documented above.

Return:

    The error message and annotation if an anomaly is detected, otherwise nil.
--]]
local function mww(name, cbuf, cfg)
    local msg = nil
    local anno = nil
    local rows, cols, ns_per_row = cbuf:get_configuration()
    ns_per_row = ns_per_row * 1e9
    local current_time = cbuf:current_time()

    if cfg.win * cfg.nwin >=  rows then
        error(string.format("%s - algorithm: %s col: %d msg: arguments out of range", name, cfg.algorithm, cfg.col))
    end

    local win_size = cfg.win * ns_per_row
    local start_time = current_time - win_size
    local end_time = start_time + win_size - ns_per_row
    local mean = cbuf:compute("avg", cfg.col, start_time, end_time)
    local result = 0
    for s=start_time - win_size, current_time - win_size * cfg.nwin + 1, -win_size do
        local e = s + win_size - ns_per_row
        if cfg.trend == "decreasing" then
            if cbuf:compute("avg", cfg.col, s, e) - mean > 0 then
                local u, p = cbuf:mannwhitneyu(cfg.col, start_time, end_time, s, e)
                if p and p < cfg.pvalue then
                    result = result + 1
                end
            end
        elseif cfg.trend == "increasing" then
            if cbuf:compute("avg", cfg.col, s, e) - mean < 0 then
                local u, p = cbuf:mannwhitneyu(cfg.col, start_time, end_time, s, e)
                if p and p < cfg.pvalue then
                    result = result + 1
                end
            end
        else
            local u, p = cbuf:mannwhitneyu(cfg.col, start_time, end_time, s, e)
            if p and p < cfg.pvalue then
                result = result + 1
            end
        end
    end

    if result > cfg.nwin / 4 then
        if cfg.trend == "any" then
            msg = "detected anomaly"
        else
            msg = string.format("detected anomaly, %s values", cfg.trend)
        end

        anno = annotation.create(current_time, cfg.col, "A", msg)
        msg = string.format("%s - algorithm: %s col: %d msg: %s", name, cfg.algorithm, cfg.col, msg)
    end

    return msg, anno
end

--[[
Detect a changing trend in a non parametric data distribution using the Mann-Whitney U test.

Arguments:

- name (string)
    The name of the circular buffer

- cbuf (circular buffer userdata object)

- cfg (table)
    - algorithm (string)
        "mww_nonparametric"

    - configuration args documented above.

Return:

    The error message and annotation if an anomaly is detected, otherwise nil.
--]]
local function mww_nonparametric(name, cbuf, cfg)
    local complete_overlap = 0.5
    local msg = nil
    local anno = nil
    local rows, cols, ns_per_row = cbuf:get_configuration()
    ns_per_row = ns_per_row * 1e9
    local current_time = cbuf:current_time()

    if cfg.win * cfg.nwin >=  rows then
        error(string.format("%s - algorithm: %s col: %d msg: arguments out of range", name, cfg.algorithm, cfg.col))
    end

    local win_size = cfg.win * ns_per_row
    local start_time = current_time - win_size
    local end_time = start_time + win_size - ns_per_row
    local result = 0
    for s=start_time - win_size, current_time - win_size * cfg.nwin + 1, -win_size do
        local e = s + win_size - ns_per_row
        local u, p = cbuf:mannwhitneyu(cfg.col, start_time, end_time, s, e)
        if u then
            result = result + u / (cfg.win * cfg.win) - complete_overlap
        end
    end

    local pstat = result / (cfg.nwin - 1) + complete_overlap
    if cfg.pstat < complete_overlap and pstat < cfg.pstat
    or cfg.pstat > complete_overlap and pstat > cfg.pstat then
        msg = string.format("detected anomaly, pstat: %G", pstat)
        anno = annotation.create(current_time, cfg.col, "A", msg)
        msg = string.format("%s - algorithm: %s col: %d msg: %s", name, cfg.algorithm, cfg.col, msg)
    end

    return msg, anno
end


--[[
Detect a significant rate of change when compared with historical data. Compute
the average of the last X intervals and the X intervals before that and compare
the difference against the historical window's standard deviation.

Arguments:

- name (string)
    The name of the circular buffer

- cbuf (circular buffer userdata objcet)

- cfg (table)
    - algorithm (string)
        "roc"

    - configuration args documented above.


Return:

    The error message and annotation if an anomaly is detected, otherwise nil.

--]]
local function roc(name, cbuf, cfg)
    local msg = nil
    local anno = nil
    local rows, cols, ns_per_row = cbuf:get_configuration()
    ns_per_row = ns_per_row * 1e9
    local current_time = cbuf:current_time()

    if 3 * cfg.win >= rows or (cfg.hwin > 0 and cfg.hwin < cfg.win) then
        error(string.format("%s - algorithm: %s col: %d msg: win=%d hwin=%d arguments out of range (rows=%d)",
                            name, cfg.algorithm, cfg.col, cfg.win, cfg.hwin, rows))
    end

    local sliding_window = ns_per_row * cfg.win
    local ps = current_time - sliding_window * 2
    local cs = current_time - sliding_window
    local hs = nil
    local pe = cs - ns_per_row
    local ce = current_time - ns_per_row
    local he = ps - ns_per_row

    if cfg.hwin ~= 0 then
        hs = ps - cfg.hwin * ns_per_row
    end

    local historical_sd, hsamples = cbuf:compute("sd" , cfg.col, hs, he)
    local previous_avg, psamples  = cbuf:compute("avg", cfg.col, ps, pe)
    local current_avg, csamples   = cbuf:compute("avg", cfg.col, cs, ce)

    local loss_of_data = nil
    if cfg.loss_of_data then
        loss_of_data = (psamples > 0 and csamples == 0)
    end

    local start_of_data = nil
    if cfg.start_of_data then
        start_of_data = (psamples == 0 and csamples > 0)
    end

    local delta = math.abs(current_avg - previous_avg)
    if delta > historical_sd * cfg.sd
    or loss_of_data or start_of_data then
        if loss_of_data then
            msg = "no new data"
        elseif start_of_data then
            msg = "unexpected data"
        else
            msg = string.format("detected anomaly, standard deviation exceeds %G", cfg.sd)
        end
        anno = annotation.create(current_time, cfg.col, "A", msg)
        msg = string.format("%s - algorithm: %s col: %d msg: %s", name, cfg.algorithm, cfg.col, msg)
    end

    return msg, anno
end


local function build_config_table(t, name, args)
    if not t[name] then
        t[name] = {args}
    else
        table.insert(t[name], args)
    end
    return t
end

--[[ Public Interface --]]

function parse_config(config)
    if not config then return end

    local sep = l.P"," * l.space^0
    local win = sep * l.Cg(l.digit^1 / tonumber, "win")
    local col = l.P"," * l.space^0 * l.Cg(l.digit^1 / tonumber, "col")
    local name = l.P'"' * l.Cs(((l.P(1) - '"') + l.P'""' / '"')^1) * '"'

    local tf = l.P"true" / function() return true end + l.P"false" / function() return false end
    local start_of_data = sep * l.Cg(tf, "start_of_data")
    local loss_of_data = sep*  l.Cg(tf, "loss_of_data")
    local sd = sep * l.Cg(l.digit^1 * (l.P"." * l.digit^1)^-1 / tonumber, "sd")
    local hwin = sep * l.Cg(l.digit^1 / tonumber, "hwin")
    local roc_args = l.Ct(col * win * hwin * sd * loss_of_data * start_of_data * l.Cg(l.Cc("roc"), "algorithm"))
    local roc = l.space^0 * "roc(" * name * roc_args * ")"

    local nwin = sep * l.Cg(l.digit^1 / tonumber, "nwin")
    local pvalue = sep * l.Cg(l.digit^1 * (l.P"." * l.digit^1)^-1 / tonumber, "pvalue")
    local pstat = sep * l.Cg(l.P"0" * (l.P"." * l.digit^1)^-1 / tonumber, "pstat")
    local trend = sep * l.Cg(l.P"decreasing" + "increasing" + "any", "trend")
    local mww_args = l.Ct(col * win * nwin * pvalue * trend * l.Cg(l.Cc("mww"), "algorithm"))
    local mww = l.space^0 * "mww(" * name * mww_args * ")"
    local mww_noparametric_args = l.Ct(col * win * nwin * pstat * l.Cg(l.Cc("mww_nonparametric"), "algorithm"))
    local mww_nonparametric = l.space^0 * "mww_nonparametric(" * name * mww_noparametric_args * ")"

    local grammar = l.Cf(l.Ct"" * l.Cg(roc + mww_nonparametric + mww)^1, build_config_table) * -1

    local c = grammar:match(config)
    if not c then
        error("could not parse the anomaly_config")
    end

    return c
end


function detect(ns, name, cbuf, anomaly_config)
    if not cbuf:get(ns, 1) then
        cbuf:add(ns, 1, 0/0) -- always advance the buffer/graph
    end

    local t = anomaly_config[name]
    if not t then return end

    if not t.last_run then
        t.last_run = 0
        local rows, cols, spr = cbuf:get_configuration()
        t.interval = spr * 1e9
    end

    -- Only run the test if the graph has advanced (the graph and ticker
    -- interval don't have to be in sync).
    if ns > t.last_run and ns - t.last_run >= t.interval then
        t.last_run = ns
    else
        return
    end

    local msg, anno, annos, msgs = nil, nil, {}, {}
    for i, cfg in ipairs(t) do
        if cfg.algorithm == "roc" then
            msg, anno = roc(name, cbuf, cfg)
        elseif cfg.algorithm == "mww" then
            msg, anno = mww(name, cbuf, cfg)
        elseif cfg.algorithm == "mww_nonparametric" then
            msg, anno = mww_nonparametric(name, cbuf, cfg)
        else
            error(string.format("%s - algorithm: %s col: %d msg: unknown algorithm", name, cfg.algorithm, cfg.col))
        end
        if msg then
            table.insert(msgs, msg)
            table.insert(annos, anno)
        end
    end

    if msgs[1] then
        return table.concat(msgs, "\n"), annos
    end

    return
end

return M
