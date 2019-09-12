import * as $protobuf from "protobufjs";
/** Properties of an Event. */
export interface IEvent {

    /** Event entity */
    entity?: (string|null);

    /** Event op */
    op?: (string|null);

    /** Event data */
    data?: (string|null);

    /** Event id */
    id?: (string|null);
}

/** Represents an Event. */
export class Event implements IEvent {

    /**
     * Constructs a new Event.
     * @param [properties] Properties to set
     */
    constructor(properties?: IEvent);

    /** Event entity. */
    public entity: string;

    /** Event op. */
    public op: string;

    /** Event data. */
    public data: string;

    /** Event id. */
    public id: string;

    /**
     * Creates a new Event instance using the specified properties.
     * @param [properties] Properties to set
     * @returns Event instance
     */
    public static create(properties?: IEvent): Event;

    /**
     * Encodes the specified Event message. Does not implicitly {@link Event.verify|verify} messages.
     * @param message Event message or plain object to encode
     * @param [writer] Writer to encode to
     * @returns Writer
     */
    public static encode(message: IEvent, writer?: $protobuf.Writer): $protobuf.Writer;

    /**
     * Encodes the specified Event message, length delimited. Does not implicitly {@link Event.verify|verify} messages.
     * @param message Event message or plain object to encode
     * @param [writer] Writer to encode to
     * @returns Writer
     */
    public static encodeDelimited(message: IEvent, writer?: $protobuf.Writer): $protobuf.Writer;

    /**
     * Decodes an Event message from the specified reader or buffer.
     * @param reader Reader or buffer to decode from
     * @param [length] Message length if known beforehand
     * @returns Event
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): Event;

    /**
     * Decodes an Event message from the specified reader or buffer, length delimited.
     * @param reader Reader or buffer to decode from
     * @returns Event
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): Event;

    /**
     * Verifies an Event message.
     * @param message Plain object to verify
     * @returns `null` if valid, otherwise the reason why it is not
     */
    public static verify(message: { [k: string]: any }): (string|null);

    /**
     * Creates an Event message from a plain object. Also converts values to their respective internal types.
     * @param object Plain object
     * @returns Event
     */
    public static fromObject(object: { [k: string]: any }): Event;

    /**
     * Creates a plain object from an Event message. Also converts values to other types if specified.
     * @param message Event
     * @param [options] Conversion options
     * @returns Plain object
     */
    public static toObject(message: Event, options?: $protobuf.IConversionOptions): { [k: string]: any };

    /**
     * Converts this Event to JSON.
     * @returns JSON object
     */
    public toJSON(): { [k: string]: any };
}

/** Properties of a Request. */
export interface IRequest {

    /** Request id */
    id?: (string|null);

    /** Request entity */
    entity?: (string|null);

    /** Request target */
    target?: (string|null);
}

/** Represents a Request. */
export class Request implements IRequest {

    /**
     * Constructs a new Request.
     * @param [properties] Properties to set
     */
    constructor(properties?: IRequest);

    /** Request id. */
    public id: string;

    /** Request entity. */
    public entity: string;

    /** Request target. */
    public target: string;

    /**
     * Creates a new Request instance using the specified properties.
     * @param [properties] Properties to set
     * @returns Request instance
     */
    public static create(properties?: IRequest): Request;

    /**
     * Encodes the specified Request message. Does not implicitly {@link Request.verify|verify} messages.
     * @param message Request message or plain object to encode
     * @param [writer] Writer to encode to
     * @returns Writer
     */
    public static encode(message: IRequest, writer?: $protobuf.Writer): $protobuf.Writer;

    /**
     * Encodes the specified Request message, length delimited. Does not implicitly {@link Request.verify|verify} messages.
     * @param message Request message or plain object to encode
     * @param [writer] Writer to encode to
     * @returns Writer
     */
    public static encodeDelimited(message: IRequest, writer?: $protobuf.Writer): $protobuf.Writer;

    /**
     * Decodes a Request message from the specified reader or buffer.
     * @param reader Reader or buffer to decode from
     * @param [length] Message length if known beforehand
     * @returns Request
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): Request;

    /**
     * Decodes a Request message from the specified reader or buffer, length delimited.
     * @param reader Reader or buffer to decode from
     * @returns Request
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): Request;

    /**
     * Verifies a Request message.
     * @param message Plain object to verify
     * @returns `null` if valid, otherwise the reason why it is not
     */
    public static verify(message: { [k: string]: any }): (string|null);

    /**
     * Creates a Request message from a plain object. Also converts values to their respective internal types.
     * @param object Plain object
     * @returns Request
     */
    public static fromObject(object: { [k: string]: any }): Request;

    /**
     * Creates a plain object from a Request message. Also converts values to other types if specified.
     * @param message Request
     * @param [options] Conversion options
     * @returns Plain object
     */
    public static toObject(message: Request, options?: $protobuf.IConversionOptions): { [k: string]: any };

    /**
     * Converts this Request to JSON.
     * @returns JSON object
     */
    public toJSON(): { [k: string]: any };
}

/** Properties of a DocHeaders. */
export interface IDocHeaders {

    /** DocHeaders id */
    id?: (string|null);

    /** DocHeaders docHeaders */
    docHeaders?: (IDocHeader[]|null);
}

/** Represents a DocHeaders. */
export class DocHeaders implements IDocHeaders {

    /**
     * Constructs a new DocHeaders.
     * @param [properties] Properties to set
     */
    constructor(properties?: IDocHeaders);

    /** DocHeaders id. */
    public id: string;

    /** DocHeaders docHeaders. */
    public docHeaders: IDocHeader[];

    /**
     * Creates a new DocHeaders instance using the specified properties.
     * @param [properties] Properties to set
     * @returns DocHeaders instance
     */
    public static create(properties?: IDocHeaders): DocHeaders;

    /**
     * Encodes the specified DocHeaders message. Does not implicitly {@link DocHeaders.verify|verify} messages.
     * @param message DocHeaders message or plain object to encode
     * @param [writer] Writer to encode to
     * @returns Writer
     */
    public static encode(message: IDocHeaders, writer?: $protobuf.Writer): $protobuf.Writer;

    /**
     * Encodes the specified DocHeaders message, length delimited. Does not implicitly {@link DocHeaders.verify|verify} messages.
     * @param message DocHeaders message or plain object to encode
     * @param [writer] Writer to encode to
     * @returns Writer
     */
    public static encodeDelimited(message: IDocHeaders, writer?: $protobuf.Writer): $protobuf.Writer;

    /**
     * Decodes a DocHeaders message from the specified reader or buffer.
     * @param reader Reader or buffer to decode from
     * @param [length] Message length if known beforehand
     * @returns DocHeaders
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): DocHeaders;

    /**
     * Decodes a DocHeaders message from the specified reader or buffer, length delimited.
     * @param reader Reader or buffer to decode from
     * @returns DocHeaders
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): DocHeaders;

    /**
     * Verifies a DocHeaders message.
     * @param message Plain object to verify
     * @returns `null` if valid, otherwise the reason why it is not
     */
    public static verify(message: { [k: string]: any }): (string|null);

    /**
     * Creates a DocHeaders message from a plain object. Also converts values to their respective internal types.
     * @param object Plain object
     * @returns DocHeaders
     */
    public static fromObject(object: { [k: string]: any }): DocHeaders;

    /**
     * Creates a plain object from a DocHeaders message. Also converts values to other types if specified.
     * @param message DocHeaders
     * @param [options] Conversion options
     * @returns Plain object
     */
    public static toObject(message: DocHeaders, options?: $protobuf.IConversionOptions): { [k: string]: any };

    /**
     * Converts this DocHeaders to JSON.
     * @returns JSON object
     */
    public toJSON(): { [k: string]: any };
}

/** Properties of a DocHeader. */
export interface IDocHeader {

    /** DocHeader id */
    id?: (string|null);

    /** DocHeader name */
    name?: (string|null);

    /** DocHeader root */
    root?: (string|null);

    /** DocHeader version */
    version?: (string|null);

    /** DocHeader iconName */
    iconName?: (string|null);
}

/** Represents a DocHeader. */
export class DocHeader implements IDocHeader {

    /**
     * Constructs a new DocHeader.
     * @param [properties] Properties to set
     */
    constructor(properties?: IDocHeader);

    /** DocHeader id. */
    public id: string;

    /** DocHeader name. */
    public name: string;

    /** DocHeader root. */
    public root: string;

    /** DocHeader version. */
    public version: string;

    /** DocHeader iconName. */
    public iconName: string;

    /**
     * Creates a new DocHeader instance using the specified properties.
     * @param [properties] Properties to set
     * @returns DocHeader instance
     */
    public static create(properties?: IDocHeader): DocHeader;

    /**
     * Encodes the specified DocHeader message. Does not implicitly {@link DocHeader.verify|verify} messages.
     * @param message DocHeader message or plain object to encode
     * @param [writer] Writer to encode to
     * @returns Writer
     */
    public static encode(message: IDocHeader, writer?: $protobuf.Writer): $protobuf.Writer;

    /**
     * Encodes the specified DocHeader message, length delimited. Does not implicitly {@link DocHeader.verify|verify} messages.
     * @param message DocHeader message or plain object to encode
     * @param [writer] Writer to encode to
     * @returns Writer
     */
    public static encodeDelimited(message: IDocHeader, writer?: $protobuf.Writer): $protobuf.Writer;

    /**
     * Decodes a DocHeader message from the specified reader or buffer.
     * @param reader Reader or buffer to decode from
     * @param [length] Message length if known beforehand
     * @returns DocHeader
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): DocHeader;

    /**
     * Decodes a DocHeader message from the specified reader or buffer, length delimited.
     * @param reader Reader or buffer to decode from
     * @returns DocHeader
     * @throws {Error} If the payload is not a reader or valid buffer
     * @throws {$protobuf.util.ProtocolError} If required fields are missing
     */
    public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): DocHeader;

    /**
     * Verifies a DocHeader message.
     * @param message Plain object to verify
     * @returns `null` if valid, otherwise the reason why it is not
     */
    public static verify(message: { [k: string]: any }): (string|null);

    /**
     * Creates a DocHeader message from a plain object. Also converts values to their respective internal types.
     * @param object Plain object
     * @returns DocHeader
     */
    public static fromObject(object: { [k: string]: any }): DocHeader;

    /**
     * Creates a plain object from a DocHeader message. Also converts values to other types if specified.
     * @param message DocHeader
     * @param [options] Conversion options
     * @returns Plain object
     */
    public static toObject(message: DocHeader, options?: $protobuf.IConversionOptions): { [k: string]: any };

    /**
     * Converts this DocHeader to JSON.
     * @returns JSON object
     */
    public toJSON(): { [k: string]: any };
}
