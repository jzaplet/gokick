package middleware

import (
	"gokick/app/application/bus"
	"gokick/app/domain/shared"
	"log/slog"
)

// BaseChain returns the recovery + logging + authorize + plane + tenant chain
// shared by CommandBus, QueryBus and any bus that runs user-driven commands. The
// plane and the tenant are marked right after authorization, so every handler and
// read runs with both in ctx — and the transaction the bus-specific tail opens
// (Transaction, ReadTx) lands on the matching database role and tenant scope.
// Bus-specific extras are appended by the caller.
func BaseChain(
	logger *slog.Logger,
	checker shared.PermissionChecker,
	reporter shared.ErrorReporter,
	tenantResolver shared.TenantResolver,
) []bus.Middleware {
	return []bus.Middleware{
		RecoveryMiddleware(logger, reporter),
		LoggingMiddleware(logger),
		AuthorizeMiddleware(checker),
		PlaneMiddleware(),
		TenantMiddleware(tenantResolver),
	}
}

// QueryChain is the read-side chain for the QueryBus:
// Recovery → Logging → Authorize → Plane → Tenant → ReadTx. Single source so the DI
// provider (provideQueryBus) and testfx (NewBuses) can't drift. ReadTx runs last:
// the read-only transaction it opens carries the plane and tenant marked before it
// (on SQLite it is a no-op).
func QueryChain(
	logger *slog.Logger,
	checker shared.PermissionChecker,
	reporter shared.ErrorReporter,
	tenantResolver shared.TenantResolver,
	tx shared.Transactor,
) []bus.Middleware {
	return append(BaseChain(logger, checker, reporter, tenantResolver), ReadTxMiddleware(tx))
}

// CommandChain is the full write-side chain for the CommandBus:
// Recovery → Logging → Authorize → Plane → Tenant → Audit → RunDispatcher →
// DispatchEvents → Transaction. Single source so the DI provider (provideCommandBus) and testfx
// (NewBuses) can't drift. Audit wraps OUTSIDE Transaction (failure events survive
// rollback); DispatchEvents wraps Transaction (events fire post-commit); the Run
// dispatcher sits outside Transaction (a handler's Enqueue joins the tx via
// Conn(ctx)). Interfaces only, so both the infrastructure and test layers can build it.
func CommandChain(
	logger *slog.Logger,
	checker shared.PermissionChecker,
	reporter shared.ErrorReporter,
	tenantResolver shared.TenantResolver,
	audit shared.AuditLogger,
	runDispatcher shared.RunDispatcher,
	eventBus *bus.EventBus,
	tx shared.Transactor,
) []bus.Middleware {
	return append(BaseChain(logger, checker, reporter, tenantResolver),
		AuditMiddleware(logger, audit),
		RunDispatcherMiddleware(runDispatcher),
		DispatchEventsMiddleware(logger, eventBus),
		TransactionMiddleware(logger, tx),
	)
}

// SystemChain is the middleware chain for the SystemCommandBus — the
// operator-trusted CLI commands (create-*, seed). It is the CommandBus chain
// MINUS Authorize and Tenant (no principal, no JWT-resolved tenant), and every
// command runs on the system plane (SystemPlane) — outside any tenant:
//
//	Recovery → Logging → SystemPlane → Audit → RunDispatcher → DispatchEvents → Transaction
//
// Audit wraps OUTSIDE Transaction (failure events survive rollback) and
// DispatchEvents wraps it (events fire post-commit). RunDispatcher sits between
// them, exactly as in CommandChain: this chain runs DispatchEvents, and an event
// handler is explicitly steered at RunDispatcherFromContext(ctx).Enqueue, so it
// must be present or that enqueue silently no-ops (F-008). The enqueue is a
// durable INSERT — the CLI process can exit and a serve/worker picks the run up.
// This is the SINGLE source of the chain: the DI provider (provideSystemCommandBus)
// and testfx (NewSystemBus) both call it, so the production and test buses can
// never drift. It takes interfaces only, so both the infrastructure and test
// layers can build it.
func SystemChain(
	logger *slog.Logger,
	tx shared.Transactor,
	eventBus *bus.EventBus,
	audit shared.AuditLogger,
	runDispatcher shared.RunDispatcher,
	reporter shared.ErrorReporter,
) []bus.Middleware {
	return []bus.Middleware{
		RecoveryMiddleware(logger, reporter),
		LoggingMiddleware(logger),
		SystemPlaneMiddleware(),
		AuditMiddleware(logger, audit),
		RunDispatcherMiddleware(runDispatcher),
		DispatchEventsMiddleware(logger, eventBus),
		TransactionMiddleware(logger, tx),
	}
}
