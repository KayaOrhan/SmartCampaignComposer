// Informational template estimate only. Personalization can change both the
// encoding and segment count; the server enforces the 160-rune content limit.
const basic = new Set(Array.from('@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !"#¤%&\'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà'))
const extension = new Set(Array.from('\f^{}\\[~]|€'))
export function smsEstimate(text: string): { encoding: string; segments: number } {
  const unicode = Array.from(text).some(char => !basic.has(char) && !extension.has(char))
  const units = unicode ? text.length : Array.from(text).reduce((count, char) => count + (extension.has(char) ? 2 : 1), 0)
  const single = unicode ? 70 : 160, multiple = unicode ? 67 : 153
  return { encoding: unicode ? 'Unicode' : 'GSM-7', segments: units === 0 ? 0 : units <= single ? 1 : Math.ceil(units / multiple) }
}
