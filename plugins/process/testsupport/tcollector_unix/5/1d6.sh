#!/bin/sh
#
# it's a random number generator

function roll_die() {
  # capture parameter
  declare -i DIE_SIDES=$1

  # check for die sides
  if [ ! $DIE_SIDES -gt 0 ]; then
    # default to 6
    DIE_SIDES=6
  fi
  # echo to screen
  echo $[ ( $RANDOM % $DIE_SIDES )  + 1 ]
}

for i in {1..500}
do
    roll_die 6  # roll a 1D6
done
