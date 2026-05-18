declare module 'qrcode' {
  export type QRCodeToCanvasOptions = {
    width?: number
    margin?: number
    color?: {
      dark?: string
      light?: string
    }
  }

  export function toCanvas(
    canvas: HTMLCanvasElement,
    text: string,
    options?: QRCodeToCanvasOptions,
    callback?: (error: Error | null | undefined) => void
  ): Promise<void>
}
