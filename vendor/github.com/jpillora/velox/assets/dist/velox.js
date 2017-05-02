// velox - v0.2.12 - https://github.com/jpillora/velox
// Jaime Pillora <dev@jpillora.com> - MIT Copyright 2016
(function() {
;(function (global) {

if ("EventSource" in global) return;

var reTrim = /^(\s|\u00A0)+|(\s|\u00A0)+$/g;

var EventSource = function (url) {
  var eventsource = this,
      interval = 500, // polling interval
      lastEventId = null,
      cache = '';

  if (!url || typeof url != 'string') {
    throw new SyntaxError('Not enough arguments');
  }

  this.URL = url;
  this.readyState = this.CONNECTING;
  this._pollTimer = null;
  this._xhr = null;

  function pollAgain(interval) {
    eventsource._pollTimer = setTimeout(function () {
      poll.call(eventsource);
    }, interval);
  }

  function poll() {
    try { // force hiding of the error message... insane?
      if (eventsource.readyState == eventsource.CLOSED) return;

      // NOTE: IE7 and upwards support
      var xhr = new XMLHttpRequest();
      xhr.open('GET', eventsource.URL, true);
      xhr.setRequestHeader('Accept', 'text/event-stream');
      xhr.setRequestHeader('Cache-Control', 'no-cache');
      // we must make use of this on the server side if we're working with Android - because they don't trigger
      // readychange until the server connection is closed
      xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');

      if (lastEventId != null) xhr.setRequestHeader('Last-Event-ID', lastEventId);
      cache = '';

      xhr.timeout = 50000;
      xhr.onreadystatechange = function () {
        if (this.readyState == 3 || (this.readyState == 4 && this.status == 200)) {
          // on success
          if (eventsource.readyState == eventsource.CONNECTING) {
            eventsource.readyState = eventsource.OPEN;
            eventsource.dispatchEvent('open', { type: 'open' });
          }

          var responseText = '';
          try {
            responseText = this.responseText || '';
          } catch (e) {}

          // process this.responseText
          var parts = responseText.substr(cache.length).split("\n"),
              eventType = 'message',
              data = [],
              i = 0,
              line = '';

          cache = responseText;

          // TODO handle 'event' (for buffer name), retry
          for (; i < parts.length; i++) {
            line = parts[i].replace(reTrim, '');
            if (line.indexOf('event') == 0) {
              eventType = line.replace(/event:?\s*/, '');
            } else if (line.indexOf('retry') == 0) {
              retry = parseInt(line.replace(/retry:?\s*/, ''));
              if(!isNaN(retry)) { interval = retry; }
            } else if (line.indexOf('data') == 0) {
              data.push(line.replace(/data:?\s*/, ''));
            } else if (line.indexOf('id:') == 0) {
              lastEventId = line.replace(/id:?\s*/, '');
            } else if (line.indexOf('id') == 0) { // this resets the id
              lastEventId = null;
            } else if (line == '') {
              if (data.length) {
                var event = new MessageEvent(data.join('\n'), eventsource.url, lastEventId);
                eventsource.dispatchEvent(eventType, event);
                data = [];
                eventType = 'message';
              }
            }
          }

          if (this.readyState == 4) pollAgain(interval);
          // don't need to poll again, because we're long-loading
        } else if (eventsource.readyState !== eventsource.CLOSED) {
          if (this.readyState == 4) { // and some other status
            // dispatch error
            eventsource.readyState = eventsource.CONNECTING;
            eventsource.dispatchEvent('error', { type: 'error' });
            pollAgain(interval);
          } else if (this.readyState == 0) { // likely aborted
            pollAgain(interval);
          } else {
          }
        }
      };

      xhr.send();

      setTimeout(function () {
        xhr.abort();
      }, xhr.timeout);

      eventsource._xhr = xhr;

    } catch (e) { // in an attempt to silence the errors
      eventsource.dispatchEvent('error', { type: 'error', data: e.message }); // ???
    }
  };

  poll(); // init now
};

EventSource.prototype = {
  close: function () {
    // closes the connection - disabling the polling
    this.readyState = this.CLOSED;
    clearInterval(this._pollTimer);
    this._xhr.abort();
  },
  CONNECTING: 0,
  OPEN: 1,
  CLOSED: 2,
  dispatchEvent: function (type, event) {
    var handlers = this['_' + type + 'Handlers'];
    if (handlers) {
      for (var i = 0; i < handlers.length; i++) {
        handlers[i].call(this, event);
      }
    }

    if (this['on' + type]) {
      this['on' + type].call(this, event);
    }
  },
  addEventListener: function (type, handler) {
    if (!this['_' + type + 'Handlers']) {
      this['_' + type + 'Handlers'] = [];
    }

    this['_' + type + 'Handlers'].push(handler);
  },
  removeEventListener: function (type, handler) {
    var handlers = this['_' + type + 'Handlers'];
    if (!handlers) {
      return;
    }
    for (var i = handlers.length - 1; i >= 0; --i) {
      if (handlers[i] === handler) {
        handlers.splice(i, 1);
        break;
      }
    }
  },
  onerror: null,
  onmessage: null,
  onopen: null,
  readyState: 0,
  URL: ''
};

var MessageEvent = function (data, origin, lastEventId) {
  this.data = data;
  this.origin = origin;
  this.lastEventId = lastEventId || '';
};

MessageEvent.prototype = {
  data: null,
  type: 'message',
  lastEventId: '',
  origin: ''
};

if ('module' in global) module.exports = EventSource;
global.EventSource = EventSource;

})(this);
/*!
* https://github.com/Starcounter-Jack/JSON-Patch
* json-patch-duplex.js version: 0.5.4
* (c) 2013 Joachim Wester
* MIT license
*/
var __extends = this.__extends || function (d, b) {
    for (var p in b) if (b.hasOwnProperty(p)) d[p] = b[p];
    function __() { this.constructor = d; }
    __.prototype = b.prototype;
    d.prototype = new __();
};
var OriginalError = Error;

var jsonpatch;
(function (jsonpatch) {
    /* Do nothing if module is already defined.
    Doesn't look nice, as we cannot simply put
    `!jsonpatch &&` before this immediate function call
    in TypeScript.
    */
    if (jsonpatch.apply) {
        return;
    }

    var _objectKeys = (function () {
        if (Object.keys)
            return Object.keys;

        return function (o) {
            var keys = [];
            for (var i in o) {
                if (o.hasOwnProperty(i)) {
                    keys.push(i);
                }
            }
            return keys;
        };
    })();

    function _equals(a, b) {
        switch (typeof a) {
            case 'undefined':
            case 'boolean':
            case 'string':
            case 'number':
                return a === b;
            case 'object':
                if (a === null)
                    return b === null;
                if (_isArray(a)) {
                    if (!_isArray(b) || a.length !== b.length)
                        return false;

                    for (var i = 0, l = a.length; i < l; i++)
                        if (!_equals(a[i], b[i]))
                            return false;

                    return true;
                }

                var bKeys = _objectKeys(b);
                var bLength = bKeys.length;
                if (_objectKeys(a).length !== bLength)
                    return false;

                for (var i = 0; i < bLength; i++)
                    if (!_equals(a[i], b[i]))
                        return false;

                return true;

            default:
                return false;
        }
    }

    /* We use a Javascript hash to store each
    function. Each hash entry (property) uses
    the operation identifiers specified in rfc6902.
    In this way, we can map each patch operation
    to its dedicated function in efficient way.
    */
    /* The operations applicable to an object */
    var objOps = {
        add: function (obj, key) {
            obj[key] = this.value;
            return true;
        },
        remove: function (obj, key) {
            delete obj[key];
            return true;
        },
        replace: function (obj, key) {
            obj[key] = this.value;
            return true;
        },
        move: function (obj, key, tree) {
            var temp = { op: "_get", path: this.from };
            apply(tree, [temp]);
            apply(tree, [
                { op: "remove", path: this.from }
            ]);
            apply(tree, [
                { op: "add", path: this.path, value: temp.value }
            ]);
            return true;
        },
        copy: function (obj, key, tree) {
            var temp = { op: "_get", path: this.from };
            apply(tree, [temp]);
            apply(tree, [
                { op: "add", path: this.path, value: temp.value }
            ]);
            return true;
        },
        test: function (obj, key) {
            return _equals(obj[key], this.value);
        },
        _get: function (obj, key) {
            this.value = obj[key];
        }
    };

    /* The operations applicable to an array. Many are the same as for the object */
    var arrOps = {
        add: function (arr, i) {
            arr.splice(i, 0, this.value);
            return true;
        },
        remove: function (arr, i) {
            arr.splice(i, 1);
            return true;
        },
        replace: function (arr, i) {
            arr[i] = this.value;
            return true;
        },
        move: objOps.move,
        copy: objOps.copy,
        test: objOps.test,
        _get: objOps._get
    };

    /* The operations applicable to object root. Many are the same as for the object */
    var rootOps = {
        add: function (obj) {
            rootOps.remove.call(this, obj);
            for (var key in this.value) {
                if (this.value.hasOwnProperty(key)) {
                    obj[key] = this.value[key];
                }
            }
            return true;
        },
        remove: function (obj) {
            for (var key in obj) {
                if (obj.hasOwnProperty(key)) {
                    objOps.remove.call(this, obj, key);
                }
            }
            return true;
        },
        replace: function (obj) {
            apply(obj, [
                { op: "remove", path: this.path }
            ]);
            apply(obj, [
                { op: "add", path: this.path, value: this.value }
            ]);
            return true;
        },
        move: objOps.move,
        copy: objOps.copy,
        test: function (obj) {
            return (JSON.stringify(obj) === JSON.stringify(this.value));
        },
        _get: function (obj) {
            this.value = obj;
        }
    };

    var _isArray;
    if (Array.isArray) {
        _isArray = Array.isArray;
    } else {
        _isArray = function (obj) {
            return obj.push && typeof obj.length === 'number';
        };
    }

    //3x faster than cached /^\d+$/.test(str)
    function isInteger(str) {
        var i = 0;
        var len = str.length;
        var charCode;
        while (i < len) {
            charCode = str.charCodeAt(i);
            if (charCode >= 48 && charCode <= 57) {
                i++;
                continue;
            }
            return false;
        }
        return true;
    }

    /// Apply a json-patch operation on an object tree
    function apply(tree, patches, validate) {
        var result = false, p = 0, plen = patches.length, patch, key;
        while (p < plen) {
            patch = patches[p];
            p++;

            // Find the object
            var path = patch.path || "";
            var keys = path.split('/');
            var obj = tree;

            var t = 1;
            var len = keys.length;
            var existingPathFragment = undefined;

            while (true) {
                key = keys[t].replace(/\u2603/g, "/");

                if (validate) {
                    if (existingPathFragment === undefined) {
                        if (obj[key] === undefined) {
                            existingPathFragment = keys.slice(0, t).join('/');
                        } else if (t == len - 1) {
                            existingPathFragment = patch.path;
                        }
                        if (existingPathFragment !== undefined) {
                            this.validator(patch, p - 1, tree, existingPathFragment);
                        }
                    }
                }

                t++;
                if (key === undefined) {
                    if (t >= len) {
                        result = rootOps[patch.op].call(patch, obj, key, tree); // Apply patch
                        break;
                    }
                }
                if (_isArray(obj)) {
                    if (key === '-') {
                        key = obj.length;
                    } else {
                        if (validate && !isInteger(key)) {
                            throw new JsonPatchError("Expected an unsigned base-10 integer value, making the new referenced value the array element with the zero-based index", "OPERATION_PATH_ILLEGAL_ARRAY_INDEX", p - 1, patch.path, patch);
                        }
                        key = parseInt(key, 10);
                    }
                    if (t >= len) {
                        if (validate && patch.op === "add" && key > obj.length) {
                            throw new JsonPatchError("The specified index MUST NOT be greater than the number of elements in the array", "OPERATION_VALUE_OUT_OF_BOUNDS", p - 1, patch.path, patch);
                        }
                        result = arrOps[patch.op].call(patch, obj, key, tree); // Apply patch
                        break;
                    }
                } else {
                    if (key && key.indexOf('~') != -1)
                        key = key.replace(/~1/g, '/').replace(/~0/g, '~'); // escape chars
                    if (t >= len) {
                        result = objOps[patch.op].call(patch, obj, key, tree); // Apply patch
                        break;
                    }
                }
                if(!obj) {
                  console.warning("slashes in object keys not supported");
                  break;
                }
                obj = obj[key];
            }
        }
        return result;
    }
    jsonpatch.apply = apply;

    var JsonPatchError = (function (_super) {
        __extends(JsonPatchError, _super);
        function JsonPatchError(message, name, index, operation, tree) {
            _super.call(this, message);
            this.message = message;
            this.name = name;
            this.index = index;
            this.operation = operation;
            this.tree = tree;
        }
        return JsonPatchError;
    })(OriginalError);
    jsonpatch.JsonPatchError = JsonPatchError;

    jsonpatch.Error = JsonPatchError;

    /**
    * Recursively checks whether an object has any undefined values inside.
    */
    function hasUndefined(obj) {
        if (obj === undefined) {
            return true;
        }

        if (typeof obj == "array" || typeof obj == "object") {
            for (var i in obj) {
                if (hasUndefined(obj[i])) {
                    return true;
                }
            }
        }

        return false;
    }

    /**
    * Validates a single operation. Called from `jsonpatch.validate`. Throws `JsonPatchError` in case of an error.
    * @param {object} operation - operation object (patch)
    * @param {number} index - index of operation in the sequence
    * @param {object} [tree] - object where the operation is supposed to be applied
    * @param {string} [existingPathFragment] - comes along with `tree`
    */
    function validator(operation, index, tree, existingPathFragment) {
        if (typeof operation !== 'object' || operation === null || _isArray(operation)) {
            throw new JsonPatchError('Operation is not an object', 'OPERATION_NOT_AN_OBJECT', index, operation, tree);
        } else if (!objOps[operation.op]) {
            throw new JsonPatchError('Operation `op` property is not one of operations defined in RFC-6902', 'OPERATION_OP_INVALID', index, operation, tree);
        } else if (typeof operation.path !== 'string') {
            throw new JsonPatchError('Operation `path` property is not a string', 'OPERATION_PATH_INVALID', index, operation, tree);
        } else if ((operation.op === 'move' || operation.op === 'copy') && typeof operation.from !== 'string') {
            throw new JsonPatchError('Operation `from` property is not present (applicable in `move` and `copy` operations)', 'OPERATION_FROM_REQUIRED', index, operation, tree);
        } else if ((operation.op === 'add' || operation.op === 'replace' || operation.op === 'test') && operation.value === undefined) {
            throw new JsonPatchError('Operation `value` property is not present (applicable in `add`, `replace` and `test` operations)', 'OPERATION_VALUE_REQUIRED', index, operation, tree);
        } else if ((operation.op === 'add' || operation.op === 'replace' || operation.op === 'test') && hasUndefined(operation.value)) {
            throw new JsonPatchError('Operation `value` property is not present (applicable in `add`, `replace` and `test` operations)', 'OPERATION_VALUE_CANNOT_CONTAIN_UNDEFINED', index, operation, tree);
        } else if (tree) {
            if (operation.op == "add") {
                var pathLen = operation.path.split("/").length;
                var existingPathLen = existingPathFragment.split("/").length;
                if (pathLen !== existingPathLen + 1 && pathLen !== existingPathLen) {
                    throw new JsonPatchError('Cannot perform an `add` operation at the desired path', 'OPERATION_PATH_CANNOT_ADD', index, operation, tree);
                }
            } else if (operation.op === 'replace' || operation.op === 'remove' || operation.op === '_get') {
                if (operation.path !== existingPathFragment) {
                    throw new JsonPatchError('Cannot perform the operation at a path that does not exist', 'OPERATION_PATH_UNRESOLVABLE', index, operation, tree);
                }
            } else if (operation.op === 'move' || operation.op === 'copy') {
                var existingValue = { op: "_get", path: operation.from, value: undefined };
                var error = jsonpatch.validate([existingValue], tree);
                if (error && error.name === 'OPERATION_PATH_UNRESOLVABLE') {
                    throw new JsonPatchError('Cannot perform the operation from a path that does not exist', 'OPERATION_FROM_UNRESOLVABLE', index, operation, tree);
                }
            }
        }
    }
    jsonpatch.validator = validator;

    /**
    * Validates a sequence of operations. If `tree` parameter is provided, the sequence is additionally validated against the object tree.
    * If error is encountered, returns a JsonPatchError object
    * @param sequence
    * @param tree
    * @returns {JsonPatchError|undefined}
    */
    function validate(sequence, tree) {
        try  {
            if (!_isArray(sequence)) {
                throw new JsonPatchError('Patch sequence must be an array', 'SEQUENCE_NOT_AN_ARRAY');
            }

            if (tree) {
                tree = JSON.parse(JSON.stringify(tree)); //clone tree so that we can safely try applying operations
                apply.call(this, tree, sequence, true);
            } else {
                for (var i = 0; i < sequence.length; i++) {
                    this.validator(sequence[i], i);
                }
            }
        } catch (e) {
            if (e instanceof JsonPatchError) {
                return e;
            } else {
                throw e;
            }
        }
    }
    jsonpatch.validate = validate;
})(jsonpatch || (jsonpatch = {}));

if (typeof exports !== "undefined") {
    exports.apply = jsonpatch.apply;
    exports.validate = jsonpatch.validate;
    exports.validator = jsonpatch.validator;
    exports.JsonPatchError = jsonpatch.JsonPatchError;
    exports.Error = jsonpatch.Error;
}
/* global window,WebSocket */
(function(window, document) {
  //"consts"
  var PROTO_VERISON = "v2";
  var PING_IN_INTERVAL = 45 * 1000;
  var PING_OUT_INTERVAL = 25 * 1000;
  var SLEEP_CHECK = 5 * 1000;
  var SLEEP_THRESHOLD = 30 * 1000;
  var MAX_RETRY_DELAY = 30 * 1000;
  //public method
  var velox = function(url, obj) {
    if(velox.DEFAULT === velox.SSE || !window.WebSocket)
      return velox.sse(url, obj);
    else
      return velox.ws(url, obj);
  };
  velox.WS = {ws:true};
  velox.ws = function(url, obj) {
    return new Velox(velox.WS, url, obj)
  };
  velox.SSE = velox.DEFAULT = {sse:true};
  velox.sse = function(url, obj) {
    return new Velox(velox.SSE, url, obj)
  };
  velox.proto = PROTO_VERISON;
  velox.online = true;
  //global status change handler
  //performs instant retries when the users
  //internet connection returns
  var vs = velox.connections = [];
  function onstatus(event) {
    velox.online = navigator.onLine;
    if(velox.online)
      for(var i = 0; i < vs.length; i++)
        if(vs[i].retrying)
          vs[i].retry();
  }
  window.addEventListener('online',  onstatus);
  window.addEventListener('offline', onstatus);
  //recursive merge (x <- y) - ignore $properties
  var merge = function(x, y) {
    if (!x || typeof x !== "object" || !y || typeof y !== "object")
      return y;
    var k;
    if (x instanceof Array && y instanceof Array) {
      //remove extra elements
      while (x.length > y.length)
        x.pop();
    } else {
      //remove extra properties
      for (k in x)
        if (k[0] !== "$" && !(k in y))
          delete x[k];
    }
    //iterate over either elements/properties
    for (k in y)
      x[k] = merge(x[k], y[k]);
    return x;
  };
  //helpers
  var events = ["message","error","open","close"];
  var loc = window.location;
  //velox class - represents a single websocket (Conn on the server-side)
  function Velox(type, url, obj) {
    switch(type) {
    case velox.WS:
      if(!window.WebSocket)
        throw "This browser does not support WebSockets";
      this.ws = true; break;
    case velox.SSE:
      this.sse = true; break;
    default:
      throw "Type must be velox.WS or velox.SSE";
    }
    if(!url) {
      url = "/velox";
    }
    this.url = url;
    if(!obj || typeof obj !== "object")
      throw "Invalid object";
    this.obj = obj;
    this.id = "";
    this.version = 0;
    this.onupdate = function() {/*noop*/};
    this.onerror = function() {/*noop*/};
    this.onconnect = function() {/*noop*/};
    this.ondisconnect = function() {/*noop*/};
    this.onchange = function() {/*noop*/};
    this.connected = false;
    this.connect();
  }
  Velox.prototype = {
    connect: function() {
      if(vs.indexOf(this) === -1)
        vs.push(this);
      this.retrying = true;
      this.retry();
    },
    retry: function() {
      clearTimeout(this.retry.t);
      if(this.conn)
        this.cleanup();
      if(!this.retrying)
        return;
      if(!this.delay)
        this.delay = 100;
      //set url
      var url = this.url;
      if(!(/^(ws|http)s?:/.test(url))) {
        url = loc.protocol + "//" + loc.host + url;
      }
      if(this.ws) {
        url = url.replace(/^http/, "ws");
      }
      var params = [];
      if(this.version) params.push("v="+this.version);
      if(this.id) params.push("id="+this.id);
      if(params.length) url += (/\?/.test(url) ? "&" : "?") + params.join("&");
      //connect!
      if(this.ws) {
        this.conn = new WebSocket(url);
      } else {
        this.conn = new EventSource(url, { withCredentials: true });
      }
      var _this = this;
      events.forEach(function(e) {
        _this.conn["on"+e] = _this["conn"+e].bind(_this);
      });
      this.sleepCheck.last = null;
      this.sleepCheck();
    },
    disconnect: function() {
      var i = vs.indexOf(this);
      if(i >= 0) vs.splice(i, 1);
      this.retrying = false;
      this.cleanup();
    },
    cleanup: function() {
      clearTimeout(this.pingout.t);
      if(!this.conn) {
        return;
      }
      var c = this.conn;
      this.conn = null;
      events.forEach(function(e) {
        c["on"+e] = null;
      });
      if(c && c.readyState !== c.CLOSED) {
        c.close();
      }
      this.statusCheck();
    },
    send: function(data) {
      var c = this.conn;
      if(c && c instanceof WebSocket && c.readyState === c.OPEN) {
        return c.send(data);
      }
    },
    pingin: function() {
      //ping receievd by server, reset last timer, start death timer for 45secs
      clearTimeout(this.pingin.t);
      this.pingin.t = setTimeout(this.retry.bind(this), PING_IN_INTERVAL);
    },
    pingout: function() {
      this.send("ping");
      clearTimeout(this.pingout.t);
      this.pingout.t = setTimeout(this.pingout.bind(this), PING_OUT_INTERVAL);
    },
    sleepCheck: function() {
      var data = this.sleepCheck;
      clearInterval(data.t);
      var now = Date.now();
      //should be ~5secs, over ~30sec - assume woken from sleep
      var woken = data.last && (now - data.last) > SLEEP_THRESHOLD;
      data.last = now;
      data.t = setTimeout(this.sleepCheck.bind(this), SLEEP_CHECK);
      if(woken) this.retry();
    },
    statusCheck: function(err) {
      var curr = !!this.connected;
      var next = !!(this.conn && this.conn.readyState === this.conn.OPEN);
      if(curr !== next) {
        this.connected = next;
        this.onchange(this.connected);
        if(this.connected) {
          this.onconnect();
        } else {
          this.ondisconnect();
        }
      }
    },
    connmessage: function(event) {
      var update;
      try {
        update = JSON.parse(event.data);
      } catch(err) {
        this.onerror(err);
        return;
      }
      if(update.ping) {
        this.pingin();
        return
      }
      if(update.id) {
        this.id = update.id;
      }
      if(!update.body || !this.obj) {
        this.onerror("null objects");
        return;
      }
      //perform update
      if(update.delta)
        jsonpatch.apply(this.obj, update.body);
      else
        merge(this.obj, update.body);
      //auto-angular
      if(typeof this.obj.$apply === "function")
        this.obj.$apply();
      this.onupdate(this.obj);
      this.version = update.version;
      //successful msg resets retry counter
      this.delay = 100;
    },
    connopen: function() {
      this.statusCheck();
      this.pingin(); //treat initial connection as incoming ping
      this.pingout(); //send initial ping
    },
    connclose: function() {
      this.statusCheck();
      //backoff retry connection
      this.delay = Math.min(MAX_RETRY_DELAY, this.delay*2);
      if(this.retrying && velox.online) {
        this.retry.t = setTimeout(this.connect.bind(this), this.delay);
      }
    },
    connerror: function(err) {
      if(this.conn && this.conn instanceof EventSource) {
        //eventsource has no close event - instead it has its
        //own retry mechanism. lets scrap that and simulate a close,
        //to use velox backoff retries.
        this.conn.close();
        this.connclose();
      } else {
        this.statusCheck();
        this.onerror(err);
      }
    }
  };
  //publicise
  window.velox = velox;
}(window, document, undefined));
}());
