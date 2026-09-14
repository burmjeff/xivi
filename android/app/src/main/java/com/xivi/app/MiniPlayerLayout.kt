package com.xivi.app

import android.content.Context
import android.util.AttributeSet
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import android.widget.FrameLayout
import kotlin.math.hypot

/** Handles dragging above the video surface while keeping the close button independently clickable. */
class MiniPlayerLayout @JvmOverloads constructor(context: Context, attrs: AttributeSet? = null) : FrameLayout(context, attrs) {
    var miniMode = false
    var onMove: ((Float, Float) -> Unit)? = null
    var onMoveFinished: (() -> Unit)? = null
    private val touchSlop = ViewConfiguration.get(context).scaledTouchSlop
    private var startX = 0f
    private var startY = 0f
    private var previousX = 0f
    private var previousY = 0f
    private var dragging = false
    private var touchingClose = false

    override fun onInterceptTouchEvent(event: MotionEvent): Boolean {
        if (!miniMode) return super.onInterceptTouchEvent(event)
        if (event.actionMasked == MotionEvent.ACTION_DOWN) {
            val close = findViewById<View>(R.id.player_mini_close)
            touchingClose = close.isShown && event.x >= close.left && event.x < close.right &&
                event.y >= close.top && event.y < close.bottom
        }
        return !touchingClose
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (!miniMode) return super.onTouchEvent(event)
        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                startX = event.rawX; startY = event.rawY
                previousX = startX; previousY = startY
                dragging = false
                parent.requestDisallowInterceptTouchEvent(true)
            }
            MotionEvent.ACTION_MOVE -> {
                if (!dragging && hypot(event.rawX - startX, event.rawY - startY) > touchSlop) dragging = true
                if (dragging) {
                    onMove?.invoke(event.rawX - previousX, event.rawY - previousY)
                    previousX = event.rawX; previousY = event.rawY
                }
            }
            MotionEvent.ACTION_UP -> {
                if (dragging) onMoveFinished?.invoke() else performClick()
                parent.requestDisallowInterceptTouchEvent(false)
                dragging = false
            }
            MotionEvent.ACTION_CANCEL -> {
                if (dragging) onMoveFinished?.invoke()
                parent.requestDisallowInterceptTouchEvent(false)
                dragging = false
            }
        }
        return true
    }

    override fun performClick(): Boolean {
        super.performClick()
        return true
    }
}
