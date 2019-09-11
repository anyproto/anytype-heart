import * as readline from "readline";
import * as fs from "fs";
import * as FIFO from "fifo-js";
import { spawn } from "child_process";

import { Event } from "../../protocol/build/ts/event/event";


export default class FifoPipe {

	go_temp: string = "/var/tmp/.go_pipe";
	js_temp: string = "/var/tmp/.js_pipe";
	fifo: any;

	constructor () {
		this.makeFifo();
	}

	private makeFifo () {
		const mkfifoProcess = spawn('mkfifo', [this.js_temp]);
		mkfifoProcess.on('exit', (status) => {
			if (status != 0) {
				throw new Error(`fail to create fifo with code: ${status}`);
				return
			};

			console.log('fifo created: ' + this.js_temp);
			this.fifo = new FIFO(this.js_temp);
		});
	}

	public writer (msg: any) {
		let eventMsg = Event.create(msg);
		let encoded = Event.encode(eventMsg).finish();

		let m: string = btoa(encoded.toString());
		this.fifo.write(m);
	}

	public reader (cb: any) {
		let rl = readline.createInterface({
			input: fs.createReadStream(this.go_temp)
		})

		rl.on('line', (line: string) => {
			// b64 -> msg + remove \n at the end
			const msg: string = atob(line.slice(0, -1));
			cb(Event.decode(Buffer.from(msg)));
		});
	}

	public static generateId () {
		let chars: string[] = "0123456789ABCDEF".split('');
		let len: number = 32;
		let arr: string[] = Array(len).fill(null).map(()=> chars[Math.ceil(Math.random()*chars.length) - 1]);
		return arr.join('');
	}

}
