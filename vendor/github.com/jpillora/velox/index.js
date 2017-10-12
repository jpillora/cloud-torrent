const client = require("./js/client/entry-node");
const server = require("./js/server/entry-server");
//place server functions onto the exported function
client.sync = server.sync;
client.handle = server.handle;
client.js = server.js;
//expose both client and server
module.exports = client;
