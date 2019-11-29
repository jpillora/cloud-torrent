#!/bin/bash

DIR=${CLD_DIR}
PATH=${CLD_PATH}
HASH=${CLD_HASH}
TYPE=${CLD_TYPE}
RESTAPI=${CLD_RESTAPI}
SIZE=${CLD_SIZE}

if [[ ${TYPE} == "torrent" ]]; then

    # to notify a telegram bot that a task is finished
    # see https://gist.github.com/dideler/85de4d64f66c1966788c1b2304b9caf1
    NOTIFYTEXT="${PATH} is finished!"
    /usr/bin/curl -X POST \
        -H 'Content-Type: application/json' \
        -d '{"chat_id": "123456789", "text": "'${NOTIFYTEXT}'", "disable_notification": true}' \
        https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN/sendMessage

    # to stop the task
    /usr/bin/curl --data "stop:${HASH}" "http://${RESTAPI}/api/torrent"

    # to remove the task
    /usr/bin/curl --data "delete:${HASH}" "http://${RESTAPI}/api/torrent"
fi

if [[ ${TYPE} == "file" ]] && [[ ${SIZE} -gt $((10*1024*1024)) ]]; then

    # when the file larger than 10MB, call aria2 jsonrpc to download from this server
    DOWNLOADURL=https://my-server-address/dldir/${PATH}

    /usr/bin/curl http://my.ip.address:6800/jsonrpc \
        -H "Content-Type: application/json" \
        -H "Accept: application/json" \
        --data '{"jsonrpc": "2.0","id":1, "method": "aria2.addUri", "params":["token:Just4Aria2c", ['${DOWNLOADURL}']]}'
fi