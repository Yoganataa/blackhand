package app

import (
	"go.uber.org/fx"

	"9router/proxy/internal/bootinject"
)

// BootInjectModule applies environment-driven configuration to the database
// before anything reads it.
//
// The ordering is the whole point of this module. ProvideTokenSaverConfig
// builds the token-saver configuration from the settings row, and the server
// module's invoke is what forces that constructor to run. An injection placed
// anywhere after it would write the settings row too late: the row would say
// rtkEnabled=false while the running process still had RTK on.
//
// fx runs a module's invokes in the order the modules are declared in AppModule,
// and every module's invokes run before the next module's, so declaring this
// module between DatabaseModule and ServerModule is what puts the injection
// ahead of the first reader of the settings row.
//
// It is an Invoke, not a Provide: the injection produces no value, it is a
// startup side effect. As a Provide it would be rejected for returning only an
// error.
var BootInjectModule = fx.Module("bootinject",
	fx.Invoke(bootinject.Apply),
)
