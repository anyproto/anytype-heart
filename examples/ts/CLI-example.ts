import FifoPipe from "./FifoPipe";

let fifoPipe = new FifoPipe();

fifoPipe.reader(line => console.log(line));

let i: number = 0;

let sendStandardEvent = () => {
    fifoPipe.writer({ entity:"standard", op:"test", data:String(i) })
};

setInterval(sendStandardEvent, 1000);
