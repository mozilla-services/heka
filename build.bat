@echo off
set BUILD_DIR=%CD%\build
set CTEST_OUTPUT_ON_FAILURE=1

setlocal ENABLEDELAYEDEXPANSION
set NEWGOPATH=%BUILD_DIR%\heka
if NOT "%GOBIN%"=="" (set p=!PATH:%GOBIN%;=!) else (set p=!PATH!)
endlocal & set GOPATH=%NEWGOPATH%& set GOBIN=%NEWGOPATH%\bin& set PATH=%p%;%NEWGOPATH%\bin;

if NOT exist %BUILD_DIR% mkdir %BUILD_DIR%
cd %BUILD_DIR%
cmake -DINCLUDE_MOZSVC=false -G"MinGW Makefiles" ..
mingw32-make
