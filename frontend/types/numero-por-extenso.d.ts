declare module "numero-por-extenso" {
  export const estilo: {
    readonly monetario: number
    readonly porcentagem: number
    readonly normal: number
  }
  export function porExtenso(value: number, style?: number): string
}
