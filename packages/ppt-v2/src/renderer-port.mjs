export class PptxRenderer {
  async render(_renderInput) {
    throw new Error("PptxRenderer.render must be implemented by an adapter");
  }
}
