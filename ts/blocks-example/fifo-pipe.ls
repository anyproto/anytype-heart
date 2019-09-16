require! {
	fs
	readline
	child_process : {spawn}
	'fifo-js' : FIFO
}

proto = fs.readFileSync \../../protocol.proto
protobuf = require \protobufjs

class FifoPipe
	~> @make-fifo!

	go_temp: \/var/tmp/.go_pipe
	js_temp: \/var/tmp/.js_pipe
	i: 0

	make-fifo:~>
		mkfifoProcess = spawn('mkfifo',  [@js_temp])
		mkfifoProcess.on 'exit', (code) ~>
			if (code == 0) => console.log('fifo created: ' + @js_temp)
			else console.log('fail to create fifo with code:  ' + code)
			@fifo = new FIFO @js_temp

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
			input: fs.createReadStream @go_temp

		rl.on \line, (line)~>
			root = protobuf.parse(proto, { keepCase: true }).root # or use Root#load
			Event = root.lookup("Event")
			cb(Event.decode Buffer.from atob(line.slice 0, -1))

module.exports = FifoPipe