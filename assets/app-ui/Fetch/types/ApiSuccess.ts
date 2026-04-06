export type ApiSuccess<TData> = {
    success: true;
    status: number;
    data: TData;
};
