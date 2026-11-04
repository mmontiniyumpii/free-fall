/**
 * jsdom (entorno de los tests) no implementa showModal/close de <dialog>. Este
 * sustituto mínimo solo pone y saca el atributo open y emite "close", como el
 * navegador. Foco, Esc y fondo se verifican en el navegador real.
 */
export function stubDialog(): void {
  const proto = HTMLDialogElement.prototype;
  if (typeof proto.showModal !== 'function') {
    proto.showModal = function (this: HTMLDialogElement) {
      this.setAttribute('open', '');
    };
  }
  if (typeof proto.close !== 'function') {
    proto.close = function (this: HTMLDialogElement) {
      if (!this.hasAttribute('open')) return;
      this.removeAttribute('open');
      this.dispatchEvent(new Event('close'));
    };
  }
}
