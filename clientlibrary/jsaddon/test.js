const bindings = require( 'bindings' )( 'addon' );
const com = require( '../build/ts/commands.js' );
var microtime = require( 'microtime' );

function getTime(){
	let n = microtime.nowStruct();
	
	return n[0] + "." + n[1];
}

bindings.setEventHandler( item => {
	let msg = com.anytype.Event.decode( item.data );
	if (msg.ping) {
		console.log( getTime(), "js got ping event", msg.ping.index );
	}
	
	if (msg.accountAdd) {
		service.accountSelect(
			{id: msg.accountAdd.account.id},
			(err, res) => {
				console.log( 'accountSelect err:', err, 'res:', res );
			}
		);
		
	}
} );

let toCamelCase = (str) => str[0].toUpperCase() + str.slice( 1, str.length );

let napiCall = (method, inputObj, outputObj, request, callback) => {
	let buffer = inputObj.encode( request ).finish();
	bindings.sendCommand( toCamelCase( method.name ), buffer, (item) => {
		try {
			let msg = outputObj.decode( item.data );
			console.log( "napiCall >>> got callback:", msg );
			callback( null, msg );
		} catch (err) {
			console.log( "napiCall >>> got error: ", err );
			callback( err, null );
		}
	} );
};


com.anytype.ClientCommands.prototype.rpcCall = napiCall;
let service = com.anytype.ClientCommands.create( () => {
}, false, false );


let start = new Date();

service.ping(
	{
		index: 1,
		numberOfEventsToSend: 1
	},
	(err, res) => {
		var end = new Date() - start;
		console.log( getTime(), 'js got ping rpc resp' );
	}
);

setTimeout( () => {
	
	console.log( getTime(), 'js send ping rpc req' );
	service.ping(
		{
			index: 2,
			numberOfEventsToSend: 1
		},
		(err, res) => {
			var end = new Date() - start;
			console.log( getTime(), 'js got ping rpc resp' );
		}
	);
}, 3000 );
/*
service.walletRecover({ rootPath: "/Users/roman/.anytype", mnemonic: 'input blame switch simple fatigue fragile grab goose unusual identify abuse use' }, (err, res) => {
	console.log('err:', err, 'res:', res)
});

setTimeout(function (){
	service.accountRecover({ }, (err, res) => {
		console.log('err:', err, 'res:', res)
	});
	
	
	
}, 5000);
*/
