// The import path is deliberately NOT the repo's ApiMessage path: the gate
// matches a type-only import by SHAPE, so moving or aliasing the type must
// not fail the parse.
import type { ApiMessage } from '@/fixture/wire';

// A comment line is fine.
export type DemoErrors = {
    general?: ApiMessage;
    nickname?: ApiMessage;
    email?: ApiMessage;
    role?: ApiMessage;
    rawfield?: ApiMessage;
    escaped?: ApiMessage;
    phantom?: ApiMessage;
};
