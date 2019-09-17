require! {
	fs
	readline
	child_process : {spawn}
	'fifo-js' : FIFO
}

proto = fs.readFileSync \../../protocol.proto
protobuf = require \protobufjs

class FifoPipe
	i: 0

	(@read_from, @write_to)~> 
		@make-fifo!

	make-fifo:~>
		mkfifoProcess = spawn('mkfifo',  [@write_to])
		mkfifoProcess.on 'exit', (code) ~>
			if (code == 0) => console.log('fifo created: ' + @write_to)
			else console.log('fail to create fifo with code:  ' + code)
			@fifo = new FIFO @write_to

	writer:(msg)~>
		root = protobuf.parse(proto, { keepCase: true }).root # or use Root#load
		
		
		Event = root.lookup("Event")
		eventMsg = Event.create msg
		encoded = Event.encode(eventMsg).finish()
		
		
		# console.log 'SENT->', msg

		m = btoa(encoded.toString())
		@fifo.write m

	reader:(cb)~>
		rl = readline.createInterface do
			input: fs.createReadStream @read_from

		rl.on \line, (line)~>
			root = protobuf.parse(proto, { keepCase: true }).root # or use Root#load
			Event = root.lookup("Event")
			cb(Event.decode Buffer.from atob(line.slice 0, -1))

module.exports = FifoPipe