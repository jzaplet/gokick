// Login returns ONLY a neutral `general` error by design: it is constant-time
// and never per-field, so it leaks no account-existence or lock oracle. Do NOT
// add per-field nickname/password keys here — that would invite wiring login
// into per-field errors and reintroduce exactly that oracle.
export type LoginErrors = {
    general?: string;
};
