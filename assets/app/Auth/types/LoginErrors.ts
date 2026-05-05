// general = non-field errors (auth, rate-limit, …)
// nickname / password = ValidationError with matching Field
export type LoginErrors = {
    general?: string;
    nickname?: string;
    password?: string;
};
