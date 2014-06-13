# Heka

Data Acquisition and Processing Made Easy

Heka is a tool for collecting and collating data from a number of different
sources, performing "in-flight" processing of collected data, and delivering
the results to any number of destinations for further analysis.

Heka is written in [Go](http://golang.org/), but Heka plugins can be written
in either Go or [Lua](http://lua.org). The easiest way to compile Heka is by
sourcing (see below) the build script in the root directory of the project,
which will set up a Go environment, verify the prerequisites, and install all
required dependencies. The build process also provides a mechanism for easily
integrating external plug-in packages into the generated `hekad`. For more
details and additional installation options see
[Installing](https://hekad.readthedocs.org/en/latest/installing.html).

WARNING: YOU MUST *SOURCE* THE BUILD SCRIPT (i.e. `source build.sh`) TO
         BUILD HEKA. Setting up the Go build environment requires changes to
         the shell environment, if you simply execute the script (i.e.
         `./build.sh`) these changes will not be made.
         
Resources:
* Heka project docs: http://hekad.readthedocs.org/
* GoDoc package docs: http://godoc.org/github.com/mozilla-services/heka
* Mailing list: https://mail.mozilla.org/listinfo/heka
* IRC: #heka on irc.mozilla.org
