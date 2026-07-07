#!/bin/bash

OFFSET=20

RAPL_PATH="/sys/class/powercap/intel-rapl:0/energy_uj"

# Find AMD GPU hwmon dynamically
GPU_PATH=$(for i in /sys/class/hwmon/hwmon*; do
    if grep -q amdgpu "$i/name" 2>/dev/null; then
        echo "$i/power1_average"
        break
    fi
done)

prev_energy=$(cat $RAPL_PATH)
prev_time=$(date +%s%N)

while true; do
    sleep 1

    # --- CPU ---
    curr_energy=$(cat $RAPL_PATH)
    curr_time=$(date +%s%N)

    energy_diff=$((curr_energy - prev_energy))
    time_diff=$((curr_time - prev_time))

    cpu_power=$(awk "BEGIN {print ($energy_diff/1e6)/($time_diff/1e9)}")

    prev_energy=$curr_energy
    prev_time=$curr_time

    # --- GPU (AMD) ---
    if [ -f "$GPU_PATH" ]; then
        gpu_raw=$(cat "$GPU_PATH")
        gpu_power=$(awk "BEGIN {print $gpu_raw/1000000}")
    else
        gpu_power=0
    fi

    # --- Total ---
    total=$(awk "BEGIN {print $cpu_power + $gpu_power + $OFFSET}")

    printf "CPU: %.1f W | GPU: %.1f W | Total: %.1f W\n" "$cpu_power" "$gpu_power" "$total"
done