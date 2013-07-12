@echo off
set BUILD_DIR=%CD%\build

setlocal ENABLEDELAYEDEXPANSION
set NEWGOPATH=%BUILD_DIR%\heka
set p=!PATH:%GOPATH%\bin;=!
endlocal & set PATH=%p%;%NEWGOPATH%\bin; & set GOPATH=%NEWGOPATH%

if NOT exist %BUILD_DIR% mkdir %BUILD_DIR%
cd %BUILD_DIR%
cmake -DINCLUDE_MOZSVC=false -G"MinGW Makefiles" ..
mingw32-make
