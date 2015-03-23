gou
===

Go Utilities (logging, json, config, event helpers)

Events, for when you want sync(locked) coordination of events:

	// register to recive event
	func init() {
		RegisterEventHandler("config_afterload", func() {
			mgo_db = "test"
		})
		// automatically listen to sigquit, siqint, sigusr1, sigusr2
		RegisterEventHandler("sigquit", func() {
			Debug("shuting down, flushing data  ")
		})
	}
	
Logging, configureable command logging that can be turned up/down to show only debug,info,warn,error etc.:
	
	var logLevel *string = flag.String("logging", "debug", "Which log level: [debug,info,warn,error,fatal]")


	if logLevel == "debug" {
		log.SetFlags(log.Ltime | log.Lshortfile | log.Lmicroseconds)
		gou.SetLogger(log.New(os.Stderr, "", log.Ltime|log.Lshortfile|log.Lmicroseconds), logLevel)
	} else {
		gou.SetLogger(log.New(os.Stderr, "", log.LstdFlags), logLevel)
	}
	// logging methods
	Debug("hello", thing, " more ", stuff)

	Log(ERROR, "hello")

	Logf(ERROR, "hello %v", thing)