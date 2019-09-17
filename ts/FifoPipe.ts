import * as readline from "readline";
import * as fs from "fs";
import { spawn } from "child_process";
import { Event } from "../build/ts/protocol";
const FIFO = require("fifo-js"); // no ts-types for fifo-js

export default class FifoPipe {

	read_from: string;
	write_to: string;
	fifo: any = new FIFO();

	constructor(read_from: string, write_to: string) {
		this.read_from = read_from;
		this.write_to = write_to;
		this.makeFifo()
	}

	private makeFifo() {
		// spawn('rm', [this.write_to])
		const mkfifoProcess = spawn('mkfifo', [this.write_to]);
		mkfifoProcess.on('exit', (status) => {
			this.fifo = new FIFO(this.write_to);
			console.log('FIFO:', this.fifo, 'status:', status)
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
		// let rl = readline.createInterface({
		// 	input: fs.createReadStream(this.read_from)
		// })
		// if (!this.fifo) return;
		this.fifo.setReader( (line: string) => {
			console.log('LINE:', line)
			const msg: string = Buffer.from(line, 'base64').toString();
			cb(Event.decode(Buffer.from(msg)));
		});
	}

}
