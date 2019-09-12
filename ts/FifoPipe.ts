import * as readline from "readline";
import * as fs from "fs";
import { spawn } from "child_process";
import { Event } from "../build/ts/protocol";
const FIFO = require("fifo-js"); // no ts-types for fifo-js

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
			this.fifo = new FIFO(this.js_temp);
		});
	}

	private generateId () {
		const chars: string[] = "0123456789ABCDEF".split('');
		const len: number = 32;
		const randChar = () => chars[Math.ceil(Math.random() * chars.length) - 1];
		const arr: string[] = Array(len).fill(null).map(randChar);
		return arr.join('');
	}

	public writer (msg: any) {
		msg.id = this.generateId();
		let eventMsg = Event.create(msg);
		let encoded = Event.encode(eventMsg).finish();
		
		let m: string = Buffer.from(encoded.toString()).toString('base64')
		this.fifo.write(m);
	}

	public reader (cb: any) {
		let rl = readline.createInterface({
			input: fs.createReadStream(this.go_temp)
		})

		rl.on('line', (line: string) => {
			const msg: string = Buffer.from(line.slice(0, -1), 'base64').toString();
			cb(Event.decode(Buffer.from(msg)));
		});
	}

}
