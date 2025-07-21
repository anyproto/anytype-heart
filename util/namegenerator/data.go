package namegenerator

//nolint:funlen
func getAdjectives() []string {
	return []string{
		"Absurd", "Amusing", "Antsy", "Awkward", "Baffled", "Baggy", "Bamboozled", "Bananas", "Bashful", "Batty",
		"Bawling", "Bearded", "Bewildered", "Bizarre", "Blubbering", "Boisterous", "Bonkers", "Bouncy", "Brainy", "Breezy",
		"Bristly", "Broken", "Bubbly", "Buff", "Bumbling", "Bumpy", "Burpy", "Cheeky", "Cheesy", "Chunky",
		"Clumsy", "Confused", "Crabby", "Cranky", "Crazy", "Crunchy", "Curly", "Curious", "Daffy", "Dandy",
		"Dapper", "Darting", "Dazed", "Delirious", "Derpy", "Dizzy", "Dopey", "Drifty", "Drippy", "Droopy",
		"Dummy", "Dunno", "Eccentric", "Electric", "Energetic", "Excitable", "Fidgety", "Fizzy", "Flaky", "Flamboyant",
		"Flashy", "Fluffy", "Flustered", "Fretful", "Frothy", "Funky", "Fuzzy", "Giddy", "Glitchy", "Gloppy",
		"Goofy", "Grinning", "Grizzly", "Grouchy", "Grubby", "Grumpy", "Hairy", "Haphazard", "Hectic", "Hilarious",
		"Hyper", "Icky", "Infamous", "Inky", "Itchy", "Janky", "Jiggly", "Jolly", "Jumpy", "Kooky",
		"Krazy", "Lazy", "Leaky", "Loony", "Loopy", "Lopsided", "Loud", "Lumpy", "Madcap", "Mangy",
		"Maniacal", "Messy", "Mindless", "Mischievous", "Mismatched", "Moldy", "Monkeyish", "Moody", "Moosey", "Muddy",
		"Napping", "Nerdy", "Nifty", "Nosy", "Nutty", "Odd", "Offbeat", "Oinky", "Outlandish", "Peppy",
		"Perky", "Pickled", "Picky", "Pipsqueak", "Piquant", "Plucky", "Poky", "Pointy", "Polar", "Pompous",
		"Pouncy", "Pranky", "Puzzled", "Quacked", "Quacky", "Quirky", "Ragged", "Rambunctious", "Rascally", "Ridiculous",
		"Rowdy", "Rusty", "Saucy", "Scatterbrained", "Scrappy", "Scratchy", "Screechy", "Scruffy", "Shaggy", "Shaky",
		"Sheepish", "Shifty", "Shocked", "Shonky", "Shrimpy", "Silly", "Sizzling", "Sketchy", "Skittish", "Slaphappy",
		"Sleepy", "Slippery", "Sloppy", "Slowpoke", "Smelly", "Snarky", "Sneaky", "Snoozy", "Snuggly", "Soapy",
		"Spiffy", "Spiky", "Spinny", "Spooky", "Spunky", "Squeaky", "Squishy", "Stinky", "Sticky",
		"Stompy", "Stubby", "Stuffy", "Swanky", "Sweaty", "Swirly", "Tangy", "Tatty", "Teeny", "Thirsty",
		"Ticklish", "Tipsy", "Toasty", "Toothy", "Tooty", "Topsy", "Tricky", "Twinkly", "Twitchy", "Unhinged",
		"Unruly", "Untidy", "Unusual", "Vacant", "Vexed", "Vibrant", "Wacky", "Waddling", "Warty", "Wavy",
		"Weepy", "Whimsical", "Whiny", "Whopping", "Wiggy", "Wiggly", "Wimpy", "Wobbly", "Wonky", "Woozy",
		"Wriggly", "Wry", "Yappy", "Yawning", "Yelling", "Yodeling", "Zany", "Zappy", "Zealous", "Zippy",
	}
}

//nolint:funlen
func getNouns() []string {
	return []string{
		"Alpaca", "Anchovy", "Anvil", "Applepie", "Armadillo", "Avocado", "Axolotl", "Bacon", "Bagel", "Bandicoot",
		"Banana", "Banjo", "Beagle", "Bean", "Bearcat", "Beetle", "Biscuit", "Blobfish", "Blueberry", "Boomerang",
		"Borscht", "Burrito", "Bus", "Cabbage", "Cactus", "Calamari", "Camel", "Carrot", "Catfish", "Cauldron",
		"Cereal", "Cheesecake", "Chimichanga", "Chipmunk", "Churro", "Clam", "Clarinet", "Coconut", "Coffee", "Cookie",
		"Cornflake", "Couch", "Cowbell", "Crabapple", "Crayon", "Cricket", "Croissant", "Cucumber", "Cupcake", "Dango",
		"Dingo", "Dirigible", "Dodo", "Donkey", "Donut", "Dragonfruit", "Drumstick", "Dumpling", "Eggplant", "Elephant",
		"Elk", "Enchilada", "Ferret", "Fiddle", "Firetruck", "Flamingo", "Flapjack", "Floss", "Fork", "Fridge",
		"Fritter", "Fugu", "Gator", "Gazelle", "Gecko", "Giraffe", "Gnome", "Goblin", "Gopher", "Grapefruit",
		"Gravyboat", "Grenade", "Guitar", "Gumdrop", "Hamster", "Hedgehog", "Helicopter", "Honeybadger", "Hotdog", "Housecat",
		"Hula", "Iguana", "Iron", "Jalapeno", "Jellybean", "Jellyfish", "Kangaroo", "Kazoo", "Kerfuffle", "Kiwi",
		"Koala", "Kombucha", "Krispy", "Llama", "Lobster", "Lollipop", "Lychee", "Macaroon", "Magpie", "Mango",
		"Manatee", "Marshmallow", "Marmot", "Matador", "Meatball", "Melon", "Microwave", "Milkshake", "Minivan", "Monkey",
		"Moose", "Mosquito", "Moth", "Muffin", "Mug", "Mushroom", "Nacho", "Noodle", "Nuthatch", "Octopus",
		"Omelette", "Onion", "Orangutan", "Otter", "Outhouse", "Owl", "Oyster", "Pancake", "Papaya", "Parrot",
		"Peanut", "Peacock", "Pear", "Pelican", "Penguin", "Pickle", "Piglet", "Pineapple", "Piranha", "Pizza",
		"Platypus", "Plum", "PogoStick", "Pony", "Popsicle", "Porcupine", "Potato", "Pretzel", "Pudding", "Pufferfish",
		"Puffin", "Pug", "Pumpkin", "Quail", "Quokka", "Quince", "Rabbit", "Raccoon", "Radish", "Rainbow",
		"Raven", "Reindeer", "Rhubarb", "Rhinoceros", "Robot", "Rocket", "Rollerskate", "Roo", "Saguaro", "Salamander",
		"Sandwich", "Sasquatch", "Sausage", "Scooter", "Seahorse", "Seal", "Shamrock", "Shark", "Sheep", "Shipwreck",
		"Shrimp", "Skunk", "Slingshot", "Sloth", "Smore", "Snail", "Snake", "Sneaker", "Snowball", "Sock",
		"Sphinx", "Spoon", "Spatula", "Squid", "Squirrel", "Starfish", "Submarine", "Sunflower", "Sushi", "Swivelchair",
		"Taco", "Tamale", "Tapir", "Teacup", "Teapot", "Tiger", "Toad", "Tofu", "Tomato", "Tornado",
		"Tractor", "Trampoline", "Trashcan", "Treasure", "Trombone", "Tropicbird", "Truck", "Tulip", "Turkey", "Turnip",
		"Turtle", "Tyrannosaurus", "Ukulele", "Umbrella", "Unicorn", "Vacuum", "Vanilla", "Velociraptor", "Viking", "Villain",
		"Vinegar", "Violin", "Volcano", "Waffle", "Walrus", "Warthog", "Wasabi", "Watermelon", "Weasel", "Whale",
		"Wheelbarrow", "Wombat", "Wrapper", "Wrench", "Yak", "Yam", "Yeti", "Yogurt", "Yoyo", "Zebra",
		"Zeppelin", "Zucchini",
	}
}
