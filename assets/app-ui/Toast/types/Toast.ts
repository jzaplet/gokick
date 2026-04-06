import type { ToastType } from './ToastType';

export type Toast = {
    id: number;
    type: ToastType;
    message: string;
    duration: number | null;
};
