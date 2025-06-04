/**
 * Animations utility for consistent transitions and effects
 */

import { cubicIn, cubicOut, elasticOut } from 'svelte/easing';
import type { TransitionConfig } from 'svelte/transition';

/**
 * Fade transition with customizable parameters
 */
export function fade(
  node: Element,
  { delay = 0, duration = 200, easing = cubicOut }: TransitionConfig = {}
): TransitionConfig {
  const o = +getComputedStyle(node).opacity;
  
  return {
    delay,
    duration,
    easing,
    css: (t) => `opacity: ${t * o}`
  };
}

/**
 * Slide transition with customizable parameters
 */
export function slide(
  node: Element,
  { delay = 0, duration = 300, easing = cubicOut, direction = 'y', distance = 20 }: TransitionConfig & { direction?: 'x' | 'y', distance?: number } = {}
): TransitionConfig {
  const style = getComputedStyle(node);
  const opacity = +style.opacity;
  const transform = style.transform === 'none' ? '' : style.transform;
  
  const directionProps = direction === 'y'
    ? { y: distance }
    : { x: distance };
  
  return {
    delay,
    duration,
    easing,
    css: (t, u) => `
      transform: ${transform} translate${direction.toUpperCase()}(${u * directionProps[direction]}px);
      opacity: ${t * opacity};
    `
  };
}

/**
 * Scale transition with customizable parameters
 */
export function scale(
  node: Element,
  { delay = 0, duration = 250, easing = cubicOut, start = 0.95, opacity = 0 }: TransitionConfig & { start?: number, opacity?: number } = {}
): TransitionConfig {
  const style = getComputedStyle(node);
  const targetOpacity = +style.opacity;
  const transform = style.transform === 'none' ? '' : style.transform;
  
  return {
    delay,
    duration,
    easing,
    css: (t, u) => `
      transform: ${transform} scale(${start + (1 - start) * t});
      opacity: ${opacity + t * (targetOpacity - opacity)};
    `
  };
}

/**
 * Fly transition with customizable parameters
 */
export function fly(
  node: Element,
  { delay = 0, duration = 300, easing = cubicOut, x = 0, y = 0, opacity = 0 }: TransitionConfig & { x?: number, y?: number, opacity?: number } = {}
): TransitionConfig {
  const style = getComputedStyle(node);
  const targetOpacity = +style.opacity;
  const transform = style.transform === 'none' ? '' : style.transform;
  
  return {
    delay,
    duration,
    easing,
    css: (t, u) => `
      transform: ${transform} translate(${u * x}px, ${u * y}px);
      opacity: ${opacity + t * (targetOpacity - opacity)};
    `
  };
}

/**
 * Bounce transition with customizable parameters
 */
export function bounce(
  node: Element,
  { delay = 0, duration = 500, easing = elasticOut }: TransitionConfig = {}
): TransitionConfig {
  const style = getComputedStyle(node);
  const opacity = +style.opacity;
  const transform = style.transform === 'none' ? '' : style.transform;
  
  return {
    delay,
    duration,
    easing,
    css: (t) => `
      transform: ${transform} scale(${t});
      opacity: ${Math.min(t * 1.5, 1) * opacity};
    `
  };
}

/**
 * Pulse animation for drawing attention
 */
export function pulse(
  node: Element,
  { delay = 0, duration = 1000, easing = cubicInOut, intensity = 1.05 }: TransitionConfig & { intensity?: number } = {}
): TransitionConfig {
  const transform = getComputedStyle(node).transform === 'none' ? '' : getComputedStyle(node).transform;
  
  return {
    delay,
    duration,
    easing,
    css: (t) => `
      transform: ${transform} scale(${1 + (Math.sin(t * Math.PI) * 0.05 * intensity)});
    `
  };
}

/**
 * Custom cubic in-out easing function
 */
function cubicInOut(t: number): number {
  return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
}

/**
 * Apply a staggered animation to a list of elements
 * @param selector CSS selector for the elements
 * @param options Animation options
 */
export function stagger(
  selector: string,
  options: {
    delay?: number;
    staggerDelay?: number;
    duration?: number;
    easing?: (t: number) => number;
    animation?: 'fade' | 'slide' | 'scale' | 'fly' | 'bounce';
    animationParams?: Record<string, any>;
  } = {}
): void {
  const {
    delay = 0,
    staggerDelay = 50,
    duration = 300,
    easing = cubicOut,
    animation = 'fade',
    animationParams = {}
  } = options;
  
  const elements = document.querySelectorAll(selector);
  
  elements.forEach((el, index) => {
    const elementDelay = delay + index * staggerDelay;
    
    // Apply the animation based on the specified type
    switch (animation) {
      case 'fade':
        fade(el as Element, { delay: elementDelay, duration, easing, ...animationParams });
        break;
      case 'slide':
        slide(el as Element, { delay: elementDelay, duration, easing, ...animationParams });
        break;
      case 'scale':
        scale(el as Element, { delay: elementDelay, duration, easing, ...animationParams });
        break;
      case 'fly':
        fly(el as Element, { delay: elementDelay, duration, easing, ...animationParams });
        break;
      case 'bounce':
        bounce(el as Element, { delay: elementDelay, duration, easing, ...animationParams });
        break;
    }
  });
}
