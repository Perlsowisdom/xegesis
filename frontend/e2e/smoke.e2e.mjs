// The harness itself: the real app starts on the copied library and a note can be typed into.
export default [
  {
    row: "Harness: the real app opens a note and takes typing",
    shot: "00-harness.png",
    async run(t) {
      await t.openNote("");
      await t.focusEnd();
      await t.type("Typed in the real app.");
      t.assert.equal((await t.blocks())[0].text, "Typed in the real app.");
      t.assert.equal(await t.saved(), "Typed in the real app.");
    },
  },
];
