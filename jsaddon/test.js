const bindings = require( 'bindings' )( 'addon' );
var protobuf = require( "protobufjs" );
var SegfaultHandler = require( 'segfault-handler' );

SegfaultHandler.registerHandler( "crash.log" );
var EventMessage;

protobuf.load( "../pb/protos/events.proto", function (err, root){
	if (err)
		throw err;
	EventMessage = root.lookupType( "anytype.Event" );
});

bindings.setEventHandler( item => {
	console.log("got event...");
	var msg = EventMessage.decode( item.data );
	try {
		console.log("got event:  ", JSON.stringify(msg));
	}
	catch(err) {
		console.log("eventHandler error: "+ err);
	}
});


setTimeout( () => {
	
	protobuf.load( "../pb/protos/commands.proto", function (err, root){
			if (err)
				throw err;
			WalletCreateMessage = root.lookupType( "anytype.WalletCreate" );
			
			var buffer = WalletCreateMessage.encode( {pin: ""} ).finish();
			bindings.sendCommand( "WalletCreate", buffer, function (item){
				try {
					var msg = root.lookupType( "anytype.WalletCreateCallback" ).decode( item.data );
					
					console.log( "got callback: " + JSON.stringify(msg));
				}catch(err) {
					console.log(err);
				}
			});
		}
	);
}, 5000 );
