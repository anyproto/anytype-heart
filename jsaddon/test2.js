const bindings = require( 'bindings' )( 'addon' );
var protobuf = require( "protobufjs" );
var SegfaultHandler = require( 'segfault-handler' );
const com = require('../build/ts/commands.js');

SegfaultHandler.registerHandler( "crash.log" );

bindings.setEventHandler( item => {
	console.log("got event...", item);
	let msg = com.anytype.Event.decode(item.data);
	try {
		console.log("got event:", JSON.stringify(msg));
	} catch (err) {
		console.log("eventHandler error:", err);
	}
});

let toCamelCase = (str) => str[0].toUpperCase() + str.slice(1, str.length)

let napiCall = (method, inputObj, outputObj, request, callback) => { 
	let buffer = inputObj.encode(request).finish();
	bindings.sendCommand(toCamelCase(method.name), buffer, (item) => {
		try {
			let msg = outputObj.decode(item.data);
			console.log("napiCall >>> got callback:", msg);
			callback(null, msg);
		} catch (err) {
			console.log("napiCall >>> got error: ", err);
			callback(err, null);
		}
	});
}

com.anytype.ClientCommands.prototype.rpcCall = napiCall
let service = com.anytype.ClientCommands.create(() => { }, false, false);

service.walletCreate({ pin: '' }, (err, res) => {
	console.log('err:', err, 'res:', res)
})


