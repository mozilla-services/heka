# Heka

Data Acquisition and Processing Made Easy

Heka is a tool for collecting and collating data from a number of different
sources, performing "in-flight" processing of collected data, and delivering
the results to any number of destinations for further analysis.

More details can be found on the ReadTheDocs sites for the [Heka
project](http://heka-docs.readthedocs.org/) and for the [Heka
daemon](http://hekad.readthedocs.org/), and lower level package
documentation can be found on
[GoDoc](http://godoc.org/github.com/mozilla-services/heka).

Heka is written in [Go](http://golang.org/), but Heka plugins can be written
in either Go or [Lua](http://lua.org). The easiest way to install Heka is
using [Heka-Build](https://github.com/mozilla-services/heka-build), which
will build the correct version of Go, set up a Go environment, and install
all required dependencies. Heka-Build also provides a mechanism for easily
integrating external plug-in packages into the generated `hekad`.
