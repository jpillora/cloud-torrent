#!/bin/bash

# Available Variables
# - ${CLD_DIR}
# - ${CLD_PATH}
# - ${CLD_HASH}
# - ${CLD_TYPE}
# - ${CLD_RESTAPI}
# - ${CLD_SIZE}
# - ${CLD_STARTTS}
LOCALPATH="${CLD_DIR}/${CLD_PATH}"
NOWTS=$(date +%s)

# skip tasks finished too soon, more likely the program just restarted
if [[ $(($NOWTS - $CLD_STARTTS)) -le 10 ]];then
	echo "STARTTS less then 10s, should ignore this task"
	exit 0
fi

# this is called when the whole task is finished
if [[ ${CLD_TYPE} == "torrent" ]]; then

    # to notify a telegram bot that a task is finished
    # see https://gist.github.com/dideler/85de4d64f66c1966788c1b2304b9caf1
    NOTIFYTEXT="${CLD_PATH} is finished!"
    /usr/bin/curl -X POST \
        -H 'Content-Type: application/json' \
        -d '{"chat_id": "123456789", "text": "'${NOTIFYTEXT}'", "disable_notification": true}' \
        https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN/sendMessage

    # to stop the task
    /usr/bin/curl --data "stop:${CLD_HASH}" "http://${CLD_RESTAPI}/api/torrent"

    # to remove the task
    /usr/bin/curl --data "delete:${CLD_HASH}" "http://${CLD_RESTAPI}/api/torrent"
fi

# this is called when one of the files is finish, here skips files with size smaller than 10MB
if [[ ${CLD_TYPE} == "file" ]] && [[ ${CLD_SIZE} -gt $((10*1024*1024)) ]]; then

    # when the file larger than 10MB, call aria2 jsonrpc to download from this server
    DOWNLOADURL="https://my-server-address/dldir/${CLD_PATH}"

    # Exmaple: call Aria2 RPC to start a download
    /usr/bin/curl http://my.ip.address:6800/jsonrpc \
        -H "Content-Type: application/json" \
        -H "Accept: application/json" \
        --data '{"jsonrpc": "2.0","id":1, "method": "aria2.addUri", "params":["token:Just4Aria2c", ['${DOWNLOADURL}']]}'

    # Example: call rclone to upload to a remote space
	/usr/local/bin/rclone copy --log-level INFO --no-traverse "${LOCALPATH}" mydrive:/Downloads
fi

