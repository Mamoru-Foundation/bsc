#!/bin/bash
# Function to print the timestamped message
function log_timestamp() {
    echo "$(date +"%Y-%m-%d %H:%M:%S") $1"
}

log_timestamp "Stopping Geth node ..."
supervisorctl stop geth

# Wait for the geth process to stop
while supervisorctl status geth | grep -q "RUNNING"; do
    sleep 5
done

log_timestamp "Executing snapshot prune-state command..."
supervisorctl start snapshot_prune

while supervisorctl status snapshot_prune | grep -q "RUNNING"; do
    sleep 5
done

log_timestamp "Starting Geth node..."
supervisorctl start geth

log_timestamp "Geth node started successfully."

