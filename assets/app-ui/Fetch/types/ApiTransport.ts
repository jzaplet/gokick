// The response never arrived AS CONTRACTED: a network failure (status 0), a
// 2xx body that is not valid JSON, or a 2xx body that fails its generated
// guard. Deliberately carries `message` at the top level — NOT `data` — so a
// consumer cannot read it as if it were the endpoint's TError shape; narrowing
// via isTransport() is compile-enforced.
export type ApiTransport = {
    success: false;
    transport: true;
    status: number;
    message: string;
};
