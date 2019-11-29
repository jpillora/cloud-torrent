#!/bin/bash

DIR=${CLD_DIR}
PATH=${CLD_PATH}
HASH=${CLD_HASH}
TYPE=${CLD_TYPE}
RESTAPI=${CLD_RESTAPI}
SIZE=${CLD_SIZE}

NOTIFYTEXT="${PATH} is finished!"

# to notify a telegram bot
# see https://gist.github.com/dideler/85de4d64f66c1966788c1b2304b9caf1
curl -X POST \
     -H 'Content-Type: application/json' \
     -d "{\"chat_id\": \"123456789\", \"text\": \"${NOTIFYTEXT}\", \"disable_notification\": true}" \
     https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN/sendMessage


# to stop the task
/usr/bin/curl --data "stop:${HASH}" "http://${RESTAPI}/api/torrent"

# to remove the task
/usr/bin/curl --data "delete:${HASH}" "http://${RESTAPI}/api/torrent"