@echo off
call env.bat

if "%NUM_JOBS%"=="" (set NUM_JOBS=1)

if NOT exist %BUILD_DIR% mkdir %BUILD_DIR%
cd %BUILD_DIR%
cmake -DINCLUDE_MOZSVC=false -DINCLUDE_DOCKER_PLUGINS=false -DCMAKE_BUILD_TYPE=release -G"MinGW Makefiles" ..
mingw32-make -j %NUM_JOBS%
