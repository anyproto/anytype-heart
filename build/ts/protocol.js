/*eslint-disable block-scoped-var, id-length, no-control-regex, no-magic-numbers, no-prototype-builtins, no-redeclare, no-shadow, no-var, sort-vars*/
"use strict";

var $protobuf = require("protobufjs/minimal");

// Common aliases
var $Reader = $protobuf.Reader, $Writer = $protobuf.Writer, $util = $protobuf.util;

// Exported root namespace
var $root = $protobuf.roots["default"] || ($protobuf.roots["default"] = {});

$root.Event = (function() {

    /**
     * Properties of an Event.
     * @exports IEvent
     * @interface IEvent
     * @property {string|null} [entity] Event entity
     * @property {string|null} [op] Event op
     * @property {string|null} [data] Event data
     * @property {string|null} [id] Event id
     */

    /**
     * Constructs a new Event.
     * @exports Event
     * @classdesc Represents an Event.
     * @implements IEvent
     * @constructor
     * @param {IEvent=} [properties] Properties to set
     */
    function Event(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * Event entity.
     * @member {string} entity
     * @memberof Event
     * @instance
     */
    Event.prototype.entity = "";

    /**
     * Event op.
     * @member {string} op
     * @memberof Event
     * @instance
     */
    Event.prototype.op = "";

    /**
     * Event data.
     * @member {string} data
     * @memberof Event
     * @instance
     */
    Event.prototype.data = "";

    /**
     * Event id.
     * @member {string} id
     * @memberof Event
     * @instance
     */
    Event.prototype.id = "";

    /**
     * Creates a new Event instance using the specified properties.
     * @function create
     * @memberof Event
     * @static
     * @param {IEvent=} [properties] Properties to set
     * @returns {Event} Event instance
     */
    Event.create = function create(properties) {
        return new Event(properties);
    };

    /**
     * Encodes the specified Event message. Does not implicitly {@link Event.verify|verify} messages.
     * @function encode
     * @memberof Event
     * @static
     * @param {IEvent} message Event message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    Event.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.entity != null && message.hasOwnProperty("entity"))
            writer.uint32(/* id 1, wireType 2 =*/10).string(message.entity);
        if (message.op != null && message.hasOwnProperty("op"))
            writer.uint32(/* id 2, wireType 2 =*/18).string(message.op);
        if (message.data != null && message.hasOwnProperty("data"))
            writer.uint32(/* id 3, wireType 2 =*/26).string(message.data);
        if (message.id != null && message.hasOwnProperty("id"))
            writer.uint32(/* id 4, wireType 2 =*/34).string(message.id);
        return writer;
    };

    /**
     * Encodes the specified Event message, length delimited. Does not implicitly {@link Event.verify|verify} messages.
     * @function encodeDelimited
     * @memberof Event
     * @static
     * @param {IEvent} message Event message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    Event.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes an Event message from the specified reader or buffer.
     * @function decode
     * @memberof Event
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {Event} Event
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    Event.decode = function decode(reader, length) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.Event();
        while (reader.pos < end) {
            var tag = reader.uint32();
            switch (tag >>> 3) {
            case 1:
                message.entity = reader.string();
                break;
            case 2:
                message.op = reader.string();
                break;
            case 3:
                message.data = reader.string();
                break;
            case 4:
                message.id = reader.string();
                break;
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes an Event message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof Event
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {Event} Event
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    Event.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies an Event message.
     * @function verify
     * @memberof Event
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    Event.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.entity != null && message.hasOwnProperty("entity"))
            if (!$util.isString(message.entity))
                return "entity: string expected";
        if (message.op != null && message.hasOwnProperty("op"))
            if (!$util.isString(message.op))
                return "op: string expected";
        if (message.data != null && message.hasOwnProperty("data"))
            if (!$util.isString(message.data))
                return "data: string expected";
        if (message.id != null && message.hasOwnProperty("id"))
            if (!$util.isString(message.id))
                return "id: string expected";
        return null;
    };

    /**
     * Creates an Event message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof Event
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {Event} Event
     */
    Event.fromObject = function fromObject(object) {
        if (object instanceof $root.Event)
            return object;
        var message = new $root.Event();
        if (object.entity != null)
            message.entity = String(object.entity);
        if (object.op != null)
            message.op = String(object.op);
        if (object.data != null)
            message.data = String(object.data);
        if (object.id != null)
            message.id = String(object.id);
        return message;
    };

    /**
     * Creates a plain object from an Event message. Also converts values to other types if specified.
     * @function toObject
     * @memberof Event
     * @static
     * @param {Event} message Event
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    Event.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults) {
            object.entity = "";
            object.op = "";
            object.data = "";
            object.id = "";
        }
        if (message.entity != null && message.hasOwnProperty("entity"))
            object.entity = message.entity;
        if (message.op != null && message.hasOwnProperty("op"))
            object.op = message.op;
        if (message.data != null && message.hasOwnProperty("data"))
            object.data = message.data;
        if (message.id != null && message.hasOwnProperty("id"))
            object.id = message.id;
        return object;
    };

    /**
     * Converts this Event to JSON.
     * @function toJSON
     * @memberof Event
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    Event.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    return Event;
})();

$root.Request = (function() {

    /**
     * Properties of a Request.
     * @exports IRequest
     * @interface IRequest
     * @property {string|null} [id] Request id
     * @property {string|null} [entity] Request entity
     * @property {string|null} [target] Request target
     */

    /**
     * Constructs a new Request.
     * @exports Request
     * @classdesc Represents a Request.
     * @implements IRequest
     * @constructor
     * @param {IRequest=} [properties] Properties to set
     */
    function Request(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * Request id.
     * @member {string} id
     * @memberof Request
     * @instance
     */
    Request.prototype.id = "";

    /**
     * Request entity.
     * @member {string} entity
     * @memberof Request
     * @instance
     */
    Request.prototype.entity = "";

    /**
     * Request target.
     * @member {string} target
     * @memberof Request
     * @instance
     */
    Request.prototype.target = "";

    /**
     * Creates a new Request instance using the specified properties.
     * @function create
     * @memberof Request
     * @static
     * @param {IRequest=} [properties] Properties to set
     * @returns {Request} Request instance
     */
    Request.create = function create(properties) {
        return new Request(properties);
    };

    /**
     * Encodes the specified Request message. Does not implicitly {@link Request.verify|verify} messages.
     * @function encode
     * @memberof Request
     * @static
     * @param {IRequest} message Request message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    Request.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.id != null && message.hasOwnProperty("id"))
            writer.uint32(/* id 1, wireType 2 =*/10).string(message.id);
        if (message.entity != null && message.hasOwnProperty("entity"))
            writer.uint32(/* id 2, wireType 2 =*/18).string(message.entity);
        if (message.target != null && message.hasOwnProperty("target"))
            writer.uint32(/* id 3, wireType 2 =*/26).string(message.target);
        return writer;
    };

    /**
     * Encodes the specified Request message, length delimited. Does not implicitly {@link Request.verify|verify} messages.
     * @function encodeDelimited
     * @memberof Request
     * @static
     * @param {IRequest} message Request message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    Request.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a Request message from the specified reader or buffer.
     * @function decode
     * @memberof Request
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {Request} Request
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    Request.decode = function decode(reader, length) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.Request();
        while (reader.pos < end) {
            var tag = reader.uint32();
            switch (tag >>> 3) {
            case 1:
                message.id = reader.string();
                break;
            case 2:
                message.entity = reader.string();
                break;
            case 3:
                message.target = reader.string();
                break;
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a Request message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof Request
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {Request} Request
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    Request.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a Request message.
     * @function verify
     * @memberof Request
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    Request.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.id != null && message.hasOwnProperty("id"))
            if (!$util.isString(message.id))
                return "id: string expected";
        if (message.entity != null && message.hasOwnProperty("entity"))
            if (!$util.isString(message.entity))
                return "entity: string expected";
        if (message.target != null && message.hasOwnProperty("target"))
            if (!$util.isString(message.target))
                return "target: string expected";
        return null;
    };

    /**
     * Creates a Request message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof Request
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {Request} Request
     */
    Request.fromObject = function fromObject(object) {
        if (object instanceof $root.Request)
            return object;
        var message = new $root.Request();
        if (object.id != null)
            message.id = String(object.id);
        if (object.entity != null)
            message.entity = String(object.entity);
        if (object.target != null)
            message.target = String(object.target);
        return message;
    };

    /**
     * Creates a plain object from a Request message. Also converts values to other types if specified.
     * @function toObject
     * @memberof Request
     * @static
     * @param {Request} message Request
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    Request.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults) {
            object.id = "";
            object.entity = "";
            object.target = "";
        }
        if (message.id != null && message.hasOwnProperty("id"))
            object.id = message.id;
        if (message.entity != null && message.hasOwnProperty("entity"))
            object.entity = message.entity;
        if (message.target != null && message.hasOwnProperty("target"))
            object.target = message.target;
        return object;
    };

    /**
     * Converts this Request to JSON.
     * @function toJSON
     * @memberof Request
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    Request.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    return Request;
})();

$root.DocHeaders = (function() {

    /**
     * Properties of a DocHeaders.
     * @exports IDocHeaders
     * @interface IDocHeaders
     * @property {string|null} [id] DocHeaders id
     * @property {Array.<IDocHeader>|null} [docHeaders] DocHeaders docHeaders
     */

    /**
     * Constructs a new DocHeaders.
     * @exports DocHeaders
     * @classdesc Represents a DocHeaders.
     * @implements IDocHeaders
     * @constructor
     * @param {IDocHeaders=} [properties] Properties to set
     */
    function DocHeaders(properties) {
        this.docHeaders = [];
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * DocHeaders id.
     * @member {string} id
     * @memberof DocHeaders
     * @instance
     */
    DocHeaders.prototype.id = "";

    /**
     * DocHeaders docHeaders.
     * @member {Array.<IDocHeader>} docHeaders
     * @memberof DocHeaders
     * @instance
     */
    DocHeaders.prototype.docHeaders = $util.emptyArray;

    /**
     * Creates a new DocHeaders instance using the specified properties.
     * @function create
     * @memberof DocHeaders
     * @static
     * @param {IDocHeaders=} [properties] Properties to set
     * @returns {DocHeaders} DocHeaders instance
     */
    DocHeaders.create = function create(properties) {
        return new DocHeaders(properties);
    };

    /**
     * Encodes the specified DocHeaders message. Does not implicitly {@link DocHeaders.verify|verify} messages.
     * @function encode
     * @memberof DocHeaders
     * @static
     * @param {IDocHeaders} message DocHeaders message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    DocHeaders.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.id != null && message.hasOwnProperty("id"))
            writer.uint32(/* id 1, wireType 2 =*/10).string(message.id);
        if (message.docHeaders != null && message.docHeaders.length)
            for (var i = 0; i < message.docHeaders.length; ++i)
                $root.DocHeader.encode(message.docHeaders[i], writer.uint32(/* id 2, wireType 2 =*/18).fork()).ldelim();
        return writer;
    };

    /**
     * Encodes the specified DocHeaders message, length delimited. Does not implicitly {@link DocHeaders.verify|verify} messages.
     * @function encodeDelimited
     * @memberof DocHeaders
     * @static
     * @param {IDocHeaders} message DocHeaders message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    DocHeaders.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a DocHeaders message from the specified reader or buffer.
     * @function decode
     * @memberof DocHeaders
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {DocHeaders} DocHeaders
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    DocHeaders.decode = function decode(reader, length) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.DocHeaders();
        while (reader.pos < end) {
            var tag = reader.uint32();
            switch (tag >>> 3) {
            case 1:
                message.id = reader.string();
                break;
            case 2:
                if (!(message.docHeaders && message.docHeaders.length))
                    message.docHeaders = [];
                message.docHeaders.push($root.DocHeader.decode(reader, reader.uint32()));
                break;
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a DocHeaders message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof DocHeaders
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {DocHeaders} DocHeaders
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    DocHeaders.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a DocHeaders message.
     * @function verify
     * @memberof DocHeaders
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    DocHeaders.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.id != null && message.hasOwnProperty("id"))
            if (!$util.isString(message.id))
                return "id: string expected";
        if (message.docHeaders != null && message.hasOwnProperty("docHeaders")) {
            if (!Array.isArray(message.docHeaders))
                return "docHeaders: array expected";
            for (var i = 0; i < message.docHeaders.length; ++i) {
                var error = $root.DocHeader.verify(message.docHeaders[i]);
                if (error)
                    return "docHeaders." + error;
            }
        }
        return null;
    };

    /**
     * Creates a DocHeaders message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof DocHeaders
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {DocHeaders} DocHeaders
     */
    DocHeaders.fromObject = function fromObject(object) {
        if (object instanceof $root.DocHeaders)
            return object;
        var message = new $root.DocHeaders();
        if (object.id != null)
            message.id = String(object.id);
        if (object.docHeaders) {
            if (!Array.isArray(object.docHeaders))
                throw TypeError(".DocHeaders.docHeaders: array expected");
            message.docHeaders = [];
            for (var i = 0; i < object.docHeaders.length; ++i) {
                if (typeof object.docHeaders[i] !== "object")
                    throw TypeError(".DocHeaders.docHeaders: object expected");
                message.docHeaders[i] = $root.DocHeader.fromObject(object.docHeaders[i]);
            }
        }
        return message;
    };

    /**
     * Creates a plain object from a DocHeaders message. Also converts values to other types if specified.
     * @function toObject
     * @memberof DocHeaders
     * @static
     * @param {DocHeaders} message DocHeaders
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    DocHeaders.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.arrays || options.defaults)
            object.docHeaders = [];
        if (options.defaults)
            object.id = "";
        if (message.id != null && message.hasOwnProperty("id"))
            object.id = message.id;
        if (message.docHeaders && message.docHeaders.length) {
            object.docHeaders = [];
            for (var j = 0; j < message.docHeaders.length; ++j)
                object.docHeaders[j] = $root.DocHeader.toObject(message.docHeaders[j], options);
        }
        return object;
    };

    /**
     * Converts this DocHeaders to JSON.
     * @function toJSON
     * @memberof DocHeaders
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    DocHeaders.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    return DocHeaders;
})();

$root.DocHeader = (function() {

    /**
     * Properties of a DocHeader.
     * @exports IDocHeader
     * @interface IDocHeader
     * @property {string|null} [id] DocHeader id
     * @property {string|null} [name] DocHeader name
     * @property {string|null} [root] DocHeader root
     * @property {string|null} [version] DocHeader version
     * @property {string|null} [iconName] DocHeader iconName
     */

    /**
     * Constructs a new DocHeader.
     * @exports DocHeader
     * @classdesc Represents a DocHeader.
     * @implements IDocHeader
     * @constructor
     * @param {IDocHeader=} [properties] Properties to set
     */
    function DocHeader(properties) {
        if (properties)
            for (var keys = Object.keys(properties), i = 0; i < keys.length; ++i)
                if (properties[keys[i]] != null)
                    this[keys[i]] = properties[keys[i]];
    }

    /**
     * DocHeader id.
     * @member {string} id
     * @memberof DocHeader
     * @instance
     */
    DocHeader.prototype.id = "";

    /**
     * DocHeader name.
     * @member {string} name
     * @memberof DocHeader
     * @instance
     */
    DocHeader.prototype.name = "";

    /**
     * DocHeader root.
     * @member {string} root
     * @memberof DocHeader
     * @instance
     */
    DocHeader.prototype.root = "";

    /**
     * DocHeader version.
     * @member {string} version
     * @memberof DocHeader
     * @instance
     */
    DocHeader.prototype.version = "";

    /**
     * DocHeader iconName.
     * @member {string} iconName
     * @memberof DocHeader
     * @instance
     */
    DocHeader.prototype.iconName = "";

    /**
     * Creates a new DocHeader instance using the specified properties.
     * @function create
     * @memberof DocHeader
     * @static
     * @param {IDocHeader=} [properties] Properties to set
     * @returns {DocHeader} DocHeader instance
     */
    DocHeader.create = function create(properties) {
        return new DocHeader(properties);
    };

    /**
     * Encodes the specified DocHeader message. Does not implicitly {@link DocHeader.verify|verify} messages.
     * @function encode
     * @memberof DocHeader
     * @static
     * @param {IDocHeader} message DocHeader message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    DocHeader.encode = function encode(message, writer) {
        if (!writer)
            writer = $Writer.create();
        if (message.id != null && message.hasOwnProperty("id"))
            writer.uint32(/* id 1, wireType 2 =*/10).string(message.id);
        if (message.name != null && message.hasOwnProperty("name"))
            writer.uint32(/* id 2, wireType 2 =*/18).string(message.name);
        if (message.root != null && message.hasOwnProperty("root"))
            writer.uint32(/* id 3, wireType 2 =*/26).string(message.root);
        if (message.version != null && message.hasOwnProperty("version"))
            writer.uint32(/* id 4, wireType 2 =*/34).string(message.version);
        if (message.iconName != null && message.hasOwnProperty("iconName"))
            writer.uint32(/* id 5, wireType 2 =*/42).string(message.iconName);
        return writer;
    };

    /**
     * Encodes the specified DocHeader message, length delimited. Does not implicitly {@link DocHeader.verify|verify} messages.
     * @function encodeDelimited
     * @memberof DocHeader
     * @static
     * @param {IDocHeader} message DocHeader message or plain object to encode
     * @param {$protobuf.Writer} [writer] Writer to encode to
     * @returns {$protobuf.Writer} Writer
     */
    DocHeader.encodeDelimited = function encodeDelimited(message, writer) {
        return this.encode(message, writer).ldelim();
    };

    /**
     * Decodes a DocHeader message from the specified reader or buffer.
     * @function decode
     * @memberof DocHeader
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @param {number} [length] Message length if known beforehand
     * @returns {DocHeader} DocHeader
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    DocHeader.decode = function decode(reader, length) {
        if (!(reader instanceof $Reader))
            reader = $Reader.create(reader);
        var end = length === undefined ? reader.len : reader.pos + length, message = new $root.DocHeader();
        while (reader.pos < end) {
            var tag = reader.uint32();
            switch (tag >>> 3) {
            case 1:
                message.id = reader.string();
                break;
            case 2:
                message.name = reader.string();
                break;
            case 3:
                message.root = reader.string();
                break;
            case 4:
                message.version = reader.string();
                break;
            case 5:
                message.iconName = reader.string();
                break;
            default:
                reader.skipType(tag & 7);
                break;
            }
        }
        return message;
    };

    /**
     * Decodes a DocHeader message from the specified reader or buffer, length delimited.
     * @function decodeDelimited
     * @memberof DocHeader
     * @static
     * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
     * @returns {DocHeader} DocHeader
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    DocHeader.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader))
            reader = new $Reader(reader);
        return this.decode(reader, reader.uint32());
    };

    /**
     * Verifies a DocHeader message.
     * @function verify
     * @memberof DocHeader
     * @static
     * @param {Object.<string,*>} message Plain object to verify
     * @returns {string|null} `null` if valid, otherwise the reason why it is not
     */
    DocHeader.verify = function verify(message) {
        if (typeof message !== "object" || message === null)
            return "object expected";
        if (message.id != null && message.hasOwnProperty("id"))
            if (!$util.isString(message.id))
                return "id: string expected";
        if (message.name != null && message.hasOwnProperty("name"))
            if (!$util.isString(message.name))
                return "name: string expected";
        if (message.root != null && message.hasOwnProperty("root"))
            if (!$util.isString(message.root))
                return "root: string expected";
        if (message.version != null && message.hasOwnProperty("version"))
            if (!$util.isString(message.version))
                return "version: string expected";
        if (message.iconName != null && message.hasOwnProperty("iconName"))
            if (!$util.isString(message.iconName))
                return "iconName: string expected";
        return null;
    };

    /**
     * Creates a DocHeader message from a plain object. Also converts values to their respective internal types.
     * @function fromObject
     * @memberof DocHeader
     * @static
     * @param {Object.<string,*>} object Plain object
     * @returns {DocHeader} DocHeader
     */
    DocHeader.fromObject = function fromObject(object) {
        if (object instanceof $root.DocHeader)
            return object;
        var message = new $root.DocHeader();
        if (object.id != null)
            message.id = String(object.id);
        if (object.name != null)
            message.name = String(object.name);
        if (object.root != null)
            message.root = String(object.root);
        if (object.version != null)
            message.version = String(object.version);
        if (object.iconName != null)
            message.iconName = String(object.iconName);
        return message;
    };

    /**
     * Creates a plain object from a DocHeader message. Also converts values to other types if specified.
     * @function toObject
     * @memberof DocHeader
     * @static
     * @param {DocHeader} message DocHeader
     * @param {$protobuf.IConversionOptions} [options] Conversion options
     * @returns {Object.<string,*>} Plain object
     */
    DocHeader.toObject = function toObject(message, options) {
        if (!options)
            options = {};
        var object = {};
        if (options.defaults) {
            object.id = "";
            object.name = "";
            object.root = "";
            object.version = "";
            object.iconName = "";
        }
        if (message.id != null && message.hasOwnProperty("id"))
            object.id = message.id;
        if (message.name != null && message.hasOwnProperty("name"))
            object.name = message.name;
        if (message.root != null && message.hasOwnProperty("root"))
            object.root = message.root;
        if (message.version != null && message.hasOwnProperty("version"))
            object.version = message.version;
        if (message.iconName != null && message.hasOwnProperty("iconName"))
            object.iconName = message.iconName;
        return object;
    };

    /**
     * Converts this DocHeader to JSON.
     * @function toJSON
     * @memberof DocHeader
     * @instance
     * @returns {Object.<string,*>} JSON object
     */
    DocHeader.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions);
    };

    return DocHeader;
})();

module.exports = $root;
