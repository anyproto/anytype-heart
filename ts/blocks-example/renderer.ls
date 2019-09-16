$ = require \jquery
fs = require \fs
FifoPipe = require \./build/fifo-pipe.js
fifoPipe = new FifoPipe!
# setInterval (~> fifoPipe.writer("msg: #{state.i++}")), 500
fifoPipe.reader (line) -> console.log line

i = 0
@send =(i)~> fifoPipe.writer entity:"standard", op:"test", data:String(i)
setInterval (~> send 50), 500
@render=->
	switch get-route!
	case '' => render-page explorer!
	default => render-page explorer!

	$ \.link .click -> go it
	# $('body') .keydown (e)-> send entity:\io op:\keydown data:e.key

# ======================= VIEWS
@loader=-> div class:"wrapper" style="width:20%;height:20%;padding-left:40%; padding-right:50%; margin-top:25%", div class:"spinner-grow text-primary" style="width: 10rem; height: 10rem;" role:"status", span class:"sr-only", 'Loading...'

@page=(content)-> body {},
	div class:'status-line-left', "ROUTE | #{get-route!}"
	div class:'status-line-right', 'status: online'
	div class:'app',
		div class:'title-bar', 
			panel-link 'home'
			(.join '') <| map panel-link <| state.docs
		div class:'container main-content', content

@start-page=-> div class:'explorer',
	div class:'line', div class:'header', 'Select doc or create a new one'

@explorer=-> div class:'explorer',
	div class:'line', div class:'header', currentDoc!name
	div class:'btn-group',
		button class:'add-block', 'Add block' 
		button class:'save', 'Save' 
	(.join '') <| map get-block <| currentDoc!blocks

currentDoc=-> state.docs.filter(-> get-route! == it.name).0

get-block=-> div class:'block', div class:'input' contentEditable:true, it.content

@msg =-> div class:'msg', (h1 {}, &0), (p {}, &1)

@panel-link=->
	isActive = if get-route! is it.name then 'show active' else ''
	li class:"panel-link link #{isActive}" route:"#{it.name}",
		a class:" " id:"#{it.name}-merkle-tab" 'aria-controls':"#{it.name}-merkle" 'aria-selected':"false", it.name

# ======================= HELPERS
@map = (f, xs) --> [f x for x in xs]
@format-date=-> (new Date it).toLocaleDateString! + ' ' + (new Date it).toLocaleTimeString!
@params-to-str=(params)-> ["#key=\"#value\"" for key, value of params when value!=false]*" "
@xml=(tag)->(params, ...children)->
	if not params.length => "<#tag #{params-to-str(params)}>#{children*''} </#tag>"
	else "<#tag>#{params + children*''} </#tag>"
map (-> @[it]=xml it), <[ svg code hr b input dl dt dd div span a p h6 h5 h4 h3 h2 h1 button table thead tr th tbody td small ul ol li span label select option textarea form output i sub time section html head body title script footer header article link nav figure figcaption tfoot video source type iframe ]>

@get=->
	try out = JSON.parse localStorage.get-item it
	catch err
		out = localStorage.get-item it
@set=(key,val)-> localStorage.set-item key, val
@set-render=(key, val)~> localStorage.set-item(key, val); render!

@get-route=-> localStorage.get-item \route

@go=->
	r = $(it.target).attr('route')
	targ = it.target
	while (!r)
		p = $(targ).parent!
		r = p.attr('route')
		if !r => targ = p
	localStorage.set-item \route r
	render!

@render-page=-> $('body').html page it

document.addEventListener 'DOMContentLoaded', ->
	if !(localStorage.get-item 'route') then localStorage.set-item \route \home
	render! # <------ START UP

state = {
	docs: [
		{
			name: 'doc1'
			blocks: [
				{ content:'123123' }
				{ content:'456456' }
			]}, 
		{
			name: 'doc2'
			blocks: [
				{ content:'789789' }
				{ content:'AAA' }
			]
		}
	]	
}