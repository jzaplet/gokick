export type ApiError<TError> = {
    success: false;
    status: number;
    data: TError;
};
