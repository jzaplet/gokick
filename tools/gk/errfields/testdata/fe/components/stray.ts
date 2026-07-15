// An *Errors type declared outside a *Errors.ts file must be flagged; a
// type-only import mentioning an *Errors name must NOT.
import { type DemoErrors } from '../types/DemoErrors';

type StrayErrors = {
    oops?: string;
};

export const useDemo = (e: DemoErrors, s: StrayErrors): void => {
    void e;
    void s;
};
