// Draws the layers of the title cards with CoreText, in pho's own look: its
// theme colours, JetBrains Mono, a rounded box with a label in the top border
// (like pho's stack breadcrumb), a tab bar whose active tab is pho's "● Desc"
// pill, and a status-bar style key hint. compose_cast.py animates the layers.
//
// usage: swift titlecard.swift <out_dir> <spec.json>
// spec.json: {"cards": [{"name", "kind": "section"|"open"|"end", "theme": {...},
//              "title", "sub", "hints": [[key, desc]], "tabs": [..], "active",
//              "index", "count", "cmd", "url", "tagline", "version"}]}
// Writes <name>-<layer>.png per card and meta.json with the layer positions.
import AppKit
import Foundation

let W = 3840, H = 2160
let args = CommandLine.arguments
let outDir = URL(fileURLWithPath: args[1])
let spec = try JSONSerialization.jsonObject(with: Data(contentsOf: URL(fileURLWithPath: args[2]))) as! [String: Any]
try FileManager.default.createDirectory(at: outDir, withIntermediateDirectories: true)

func color(_ hex: String, _ a: CGFloat = 1) -> NSColor {
    let v = UInt32(hex.dropFirst(), radix: 16)!
    return NSColor(srgbRed: CGFloat((v >> 16) & 0xff) / 255, green: CGFloat((v >> 8) & 0xff) / 255,
                   blue: CGFloat(v & 0xff) / 255, alpha: a)
}

func mono(_ size: CGFloat, _ weight: String = "Regular") -> NSFont {
    NSFont(name: "JetBrainsMono-\(weight)", size: size) ?? NSFont.monospacedSystemFont(ofSize: size, weight: .regular)
}

// Draws into a W x H (or w x h) transparent canvas with a top-left origin.
func render(_ name: String, width: Int = W, height: Int = H, draw: () -> Void) {
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: width, pixelsHigh: height, bitsPerSample: 8,
                               samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB,
                               bytesPerRow: 0, bitsPerPixel: 0)!
    NSGraphicsContext.saveGraphicsState()
    let ctx = NSGraphicsContext(bitmapImageRep: rep)!
    ctx.cgContext.translateBy(x: 0, y: CGFloat(height))
    ctx.cgContext.scaleBy(x: 1, y: -1)
    NSGraphicsContext.current = NSGraphicsContext(cgContext: ctx.cgContext, flipped: true)
    draw()
    NSGraphicsContext.restoreGraphicsState()
    try! rep.representation(using: .png, properties: [:])!.write(to: outDir.appendingPathComponent("\(name).png"))
}

func text(_ s: String, _ font: NSFont, _ c: NSColor) -> NSAttributedString {
    NSAttributedString(string: s, attributes: [.font: font, .foregroundColor: c])
}

// Draws runs left to right from x, vertically centred on midY; returns the end x.
@discardableResult
func runs(_ parts: [NSAttributedString], x: CGFloat, midY: CGFloat) -> CGFloat {
    var x = x
    for p in parts {
        let sz = p.size()
        p.draw(at: NSPoint(x: x, y: midY - sz.height / 2))
        x += sz.width
    }
    return x
}

var meta: [String: Any] = [:]

// pho's block logo, drawn cell by cell: █ fills its cell and the double-line
// shading (═ ║ ╔ ╗ ╚ ╝) is stroked so it joins up the way a terminal draws it.
let LOGO = ["█████╗  ██╗  ██╗  █████╗", "██╔══██╗██║  ██║ ██╔══██╗", "█████╔╝ ███████║ ██║  ██║",
            "██╔══╝  ██╔══██║ ██║  ██║", "██║     ██║  ██║ ╚█████╔╝", "╚═╝     ╚═╝  ╚═╝  ╚════╝"]
let LCW: CGFloat = 62, LCH: CGFloat = 128

func drawLogo(_ c: NSColor) {
    c.setFill(); c.setStroke()
    let d = LCW * 0.2, lw = LCW * 0.11
    for (r, line) in LOGO.enumerated() {
        for (i, ch) in line.enumerated() {
            let x0 = CGFloat(i) * LCW, y0 = CGFloat(r) * LCH, x1 = x0 + LCW, y1 = y0 + LCH
            let mx = x0 + LCW / 2, my = y0 + LCH / 2
            var paths: [[NSPoint]] = []
            switch ch {
            case "█": NSRect(x: x0, y: y0, width: LCW + 0.5, height: LCH + 0.5).fill()
            case "═": paths = [[NSPoint(x: x0, y: my - d), NSPoint(x: x1, y: my - d)], [NSPoint(x: x0, y: my + d), NSPoint(x: x1, y: my + d)]]
            case "║": paths = [[NSPoint(x: mx - d, y: y0), NSPoint(x: mx - d, y: y1)], [NSPoint(x: mx + d, y: y0), NSPoint(x: mx + d, y: y1)]]
            case "╔": paths = [[NSPoint(x: mx - d, y: y1), NSPoint(x: mx - d, y: my - d), NSPoint(x: x1, y: my - d)],
                               [NSPoint(x: mx + d, y: y1), NSPoint(x: mx + d, y: my + d), NSPoint(x: x1, y: my + d)]]
            case "╗": paths = [[NSPoint(x: x0, y: my - d), NSPoint(x: mx + d, y: my - d), NSPoint(x: mx + d, y: y1)],
                               [NSPoint(x: x0, y: my + d), NSPoint(x: mx - d, y: my + d), NSPoint(x: mx - d, y: y1)]]
            case "╚": paths = [[NSPoint(x: mx - d, y: y0), NSPoint(x: mx - d, y: my + d), NSPoint(x: x1, y: my + d)],
                               [NSPoint(x: mx + d, y: y0), NSPoint(x: mx + d, y: my - d), NSPoint(x: x1, y: my - d)]]
            case "╝": paths = [[NSPoint(x: mx + d, y: y0), NSPoint(x: mx + d, y: my + d), NSPoint(x: x0, y: my + d)],
                               [NSPoint(x: mx - d, y: y0), NSPoint(x: mx - d, y: my - d), NSPoint(x: x0, y: my - d)]]
            default: break
            }
            for p in paths {
                let b = NSBezierPath(); b.move(to: p[0]); for q in p.dropFirst() { b.line(to: q) }
                b.lineWidth = lw; b.lineCapStyle = .square; b.lineJoinStyle = .miter; b.stroke()
            }
        }
    }
}

for card in spec["cards"] as! [[String: Any]] {
    let name = card["name"] as! String
    let th = card["theme"] as! [String: String]
    let bg = color(th["bg"]!), accent = color(th["accent"]!), border = color(th["border"]!)
    let textC = color(th["text"]!), bright = color(th["bright"]!), dim = color(th["dim"]!)
    let faint = color(th["faint"]!), muted = color(th["muted"]!), accentTx = color(th["accentTx"]!)
    let hintKey = color(th["hintKey"]!)
    var m: [String: Any] = [:]

    render("\(name)-bg") { bg.setFill(); NSRect(x: 0, y: 0, width: W, height: H).fill() }
    if card["kind"] as! String != "section" {
        let lw = LOGO.map { $0.count }.max()!
        render("\(name)-logo", width: Int(CGFloat(lw) * LCW) + 8, height: Int(CGFloat(LOGO.count) * LCH) + 8) { drawLogo(accent) }
    }

    // status-bar style hint: "key desc  ·  key desc", centred under midY
    func hints(_ hs: [[String]], midY: CGFloat, size: CGFloat = 40) {
        var parts: [NSAttributedString] = []
        for (i, h) in hs.enumerated() {
            if i > 0 { parts.append(text("  ·  ", mono(size), faint)) }
            parts.append(text(h[0], mono(size, "Bold"), hintKey))
            parts.append(text(" " + h[1], mono(size), dim))
        }
        let w = parts.reduce(0) { $0 + $1.size().width }
        runs(parts, x: (CGFloat(W) - w) / 2, midY: midY)
    }

    switch card["kind"] as! String {
    case "section":
        let bx: CGFloat = 640, by: CGFloat = 690, bw: CGFloat = 2560, bh: CGFloat = 660
        let tabs = card["tabs"] as! [String], active = card["active"] as! Int
        let tabFont = mono(40), pillFont = mono(40, "Bold")
        let cw = text("M", tabFont, textC).size().width
        let tabsY = by + 92, slotGap = cw * 3
        // slot i holds "n:Label" muted, or the "● Label" pill: both the same width
        var slots: [CGFloat] = []
        var x = bx + 56
        for t in tabs { slots.append(x); x += CGFloat(t.count + 2) * cw + slotGap }
        let pad: CGFloat = 16

        render("\(name)-base") {
            // box with the label set into its top border, like pho's breadcrumb
            let label = [text(" pho ", mono(38, "Bold"), accentTx), text("› ", mono(38), faint),
                         text(String(format: "%02d / %02d ", card["index"] as! Int, card["count"] as! Int), mono(38), dim)]
            let lw = label.reduce(0) { $0 + $1.size().width }
            let path = NSBezierPath(roundedRect: NSRect(x: bx, y: by, width: bw, height: bh), xRadius: 22, yRadius: 22)
            accent.setStroke(); path.lineWidth = 3.5; path.stroke()
            bg.setFill(); NSRect(x: bx + 40, y: by - 10, width: lw + 8, height: 20).fill()
            runs(label, x: bx + 44, midY: by)
            // tab bar and the rule under it
            for (i, t) in tabs.enumerated() where i != active {
                runs([text("\(i + 1):", tabFont, faint), text(t, tabFont, muted)], x: slots[i], midY: tabsY)
            }
            border.setStroke()
            let rule = NSBezierPath(); rule.move(to: NSPoint(x: bx, y: tabsY + 62)); rule.line(to: NSPoint(x: bx + bw, y: tabsY + 62))
            rule.lineWidth = 3; rule.stroke()
        }
        // the active tab: a bare highlight that slides in under the tab labels
        // from the previous section's tab, and its "● Label" text on top
        let pillText = [text("● ", pillFont, color(th["onAccent"]!)), text(tabs[active], pillFont, color(th["onAccent"]!))]
        let pw = CGFloat(tabs[active].count + 2) * cw + 2 * pad, ph: CGFloat = 64
        render("\(name)-pill", width: Int(pw.rounded(.up)), height: Int(ph)) {
            accent.setFill(); NSBezierPath(roundedRect: NSRect(x: 0, y: 0, width: pw, height: ph), xRadius: 6, yRadius: 6).fill()
        }
        render("\(name)-pilltext", width: Int(pw.rounded(.up)), height: Int(ph)) { runs(pillText, x: pad, midY: ph / 2) }
        m["pill_y"] = Int(tabsY - ph / 2)
        m["slots"] = slots.map { Int($0 - pad) }

        // title, typed in over a bg-coloured mask, with a block cursor
        let tf = mono(132, "Bold")
        let title = card["title"] as! String
        let tcw = text("M", tf, bright).size().width
        let th0 = text(title, tf, bright).size().height
        let tx = bx + 120, ty = by + 255
        render("\(name)-title", width: Int((CGFloat(title.count) * tcw).rounded(.up)) + 8, height: Int(th0.rounded(.up))) {
            text(title, tf, bright).draw(at: NSPoint(x: 0, y: 0))
        }
        render("\(name)-mask", width: Int((CGFloat(title.count) * tcw).rounded(.up)) + 16, height: Int(th0.rounded(.up))) {
            bg.setFill(); NSRect(x: 0, y: 0, width: 100000, height: 1000).fill()
        }
        let curH = tf.capHeight + 40
        render("\(name)-cursor", width: Int(tcw.rounded(.up)), height: Int(curH)) {
            accent.setFill(); NSRect(x: 0, y: 0, width: tcw, height: curH).fill()
        }
        m["title_x"] = Int(tx); m["title_y"] = Int(ty); m["char_w"] = Double(tcw); m["n"] = title.count
        m["cursor_y"] = Int(ty + (th0 - curH) / 2 + 4)

        render("\(name)-sub") {
            runs([text(card["sub"] as! String, mono(54), dim)], x: tx + 4, midY: ty + th0 + 70)
            hints(card["hints"] as! [[String]], midY: by + bh + 100)
        }

    case "open", "end":
        // the logo itself comes from agg (pho's own glyphs); these are the words around it
        let isOpen = card["kind"] as! String == "open"
        if isOpen {
            render("\(name)-tag") {
                let t = text(card["tagline"] as! String, mono(64), textC)
                runs([t], x: (CGFloat(W) - t.size().width) / 2, midY: 1430)
            }
            let v = text(card["version"] as! String, mono(44, "Bold"), accentTx)
            let vw = v.size().width + 56, vh: CGFloat = 76
            render("\(name)-ver") {
                let r = NSRect(x: (CGFloat(W) - vw) / 2, y: 1560, width: vw, height: vh)
                let p = NSBezierPath(roundedRect: r, xRadius: 38, yRadius: 38)
                accent.withAlphaComponent(0.18).setFill(); p.fill()
                accent.setStroke(); p.lineWidth = 3; p.stroke()
                runs([v], x: r.minX + 28, midY: r.midY)
            }
        } else {
            // a prompt with the install command typed in
            let cf = mono(72, "Medium")
            let cmd = card["cmd"] as! String
            let prompt = text("❯ ", cf, accentTx)
            let ccw = text("M", cf, textC).size().width
            let total = prompt.size().width + CGFloat(cmd.count) * ccw
            let cx = (CGFloat(W) - total) / 2 + prompt.size().width, cy: CGFloat = 1400
            let ch = text(cmd, cf, bright).size().height
            render("\(name)-prompt") { runs([prompt], x: cx - prompt.size().width, midY: cy) }
            render("\(name)-title", width: Int((CGFloat(cmd.count) * ccw).rounded(.up)) + 8, height: Int(ch.rounded(.up))) {
                text(cmd, cf, bright).draw(at: NSPoint(x: 0, y: 0))
            }
            render("\(name)-mask", width: Int((CGFloat(cmd.count) * ccw).rounded(.up)) + 16, height: Int(ch.rounded(.up))) {
                bg.setFill(); NSRect(x: 0, y: 0, width: 100000, height: 1000).fill()
            }
            let curH = cf.capHeight + 26
            render("\(name)-cursor", width: Int(ccw.rounded(.up)), height: Int(curH)) {
                accent.setFill(); NSRect(x: 0, y: 0, width: ccw, height: curH).fill()
            }
            m["title_x"] = Int(cx); m["title_y"] = Int(cy - ch / 2); m["char_w"] = Double(ccw); m["n"] = cmd.count
            m["cursor_y"] = Int(cy - ch / 2 + (ch - curH) / 2 + 3)
            render("\(name)-sub") {
                let u = text(card["url"] as! String, mono(48), dim)
                runs([u], x: (CGFloat(W) - u.size().width) / 2, midY: 1560)
            }
        }
    default:
        break
    }
    meta[name] = m
}
try JSONSerialization.data(withJSONObject: meta, options: [.prettyPrinted, .sortedKeys])
    .write(to: outDir.appendingPathComponent("meta.json"))
print("cards -> \(outDir.path)")
